package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"jeedesk/internal/config"
	"jeedesk/internal/hook"
	"jeedesk/internal/launcher"
	"jeedesk/internal/resolver"
	"jeedesk/internal/tray"
	"jeedesk/internal/usage"
)

const (
	blurGracePeriod     = 400 * time.Millisecond
	blurMonitorInterval = 80 * time.Millisecond
	windowSaveDelay     = 600 * time.Millisecond
	appVersion          = "1.0.0"
	recentItemsLimit    = 12
)

type toastPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// HotkeyStatus describes the current registration state of the global hotkey.
type HotkeyStatus struct {
	Hotkey     string `json:"hotkey"`
	Registered bool   `json:"registered"`
	Error      string `json:"error"`
}

// App exposes JeeDesk backend capabilities to the Wails v2 frontend.
type App struct {
	// ctx is written by OnStartup, which Wails calls on its own goroutine while the single-instance
	// wake callback may already be running. All access goes through setContext and context.
	ctxMu      sync.RWMutex
	ctx        context.Context
	baseDir    string
	configPath string
	iconsDir   string

	// cfgMu guards cfgMgr, which ReloadConfig may replace while bound methods read it.
	cfgMu         sync.RWMutex
	cfgMgr        *config.Manager
	configLoadErr error

	// usageStore counts launches for the recent list; nil when it could not be created.
	usageStore *usage.Store

	// undoMu guards the snapshots used by UndoDeleteItem and UndoDeleteGroup.
	undoMu       sync.Mutex
	deletedItem  *deletedItemSnapshot
	deletedGroup *deletedGroupSnapshot

	windowSaveMu    sync.Mutex
	windowSaveTimer *time.Timer

	activeGroupMu sync.RWMutex
	activeGroupID string

	stateMu    sync.Mutex
	visible    bool
	lastShown  time.Time
	dialogOpen atomic.Bool
	quitting   atomic.Bool
	domReadyOK atomic.Bool

	blurMu     sync.Mutex
	blurStopCh chan struct{}

	hookMu        sync.Mutex
	hotkeyMgr     *hook.HotkeyManager
	mouseMgr      *hook.MouseHookManager
	appliedHotkey string
	appliedMouse  string
	hotkeyStatus  HotkeyStatus

	errMu         sync.Mutex
	lastConfigErr string
	startupNotice string

	toastFunc       func(level, message string)
	findMainHwndFn  func() uintptr
	isVisibleFn     func(uintptr) bool
	isProcessFgFn   func() bool
	isCursorInWinFn func(uintptr) bool
	isMouseDownFn   func() bool
	focusWindowFn   func(uintptr)
}

// NewApp constructs a JeeDesk App instance rooted at baseDir.
func NewApp(baseDir string) *App {
	cleaned := filepath.Clean(baseDir)
	return &App{
		baseDir:         cleaned,
		configPath:      filepath.Join(cleaned, "config.json"),
		iconsDir:        filepath.Join(cleaned, "icons"),
		findMainHwndFn:  findMainWailsWindow,
		isVisibleFn:     isWindowVisible,
		isProcessFgFn:   isProcessForeground,
		isCursorInWinFn: isCursorInsideWindow,
		isMouseDownFn:   isMouseButtonDown,
		focusWindowFn:   focusWindowHandle,
	}
}

// loadConfig creates the config manager before any other goroutine exists, so the initial load needs
// no locking beyond the cfgMu write below. A load failure is kept for startup to report.
func (a *App) loadConfig() {
	mgr, err := config.NewManagerWithFallback(a.configPath, a.onConfigUpdated, a.onConfigError)
	a.cfgMu.Lock()
	a.cfgMgr = mgr
	a.cfgMu.Unlock()
	a.configLoadErr = err

	a.usageStore = usage.NewStore(filepath.Join(a.baseDir, "usage.json"))
}

// configManager returns the current config manager, or nil before loadConfig ran.
func (a *App) configManager() *config.Manager {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfgMgr
}

func (a *App) setConfigManager(mgr *config.Manager) {
	a.cfgMu.Lock()
	a.cfgMgr = mgr
	a.cfgMu.Unlock()
}

// hooks returns the hook managers, both nil before startup created them.
func (a *App) hooks() (*hook.HotkeyManager, *hook.MouseHookManager) {
	a.hookMu.Lock()
	defer a.hookMu.Unlock()
	return a.hotkeyMgr, a.mouseMgr
}

func (a *App) setHooks(hotkeyMgr *hook.HotkeyManager, mouseMgr *hook.MouseHookManager) {
	a.hookMu.Lock()
	a.hotkeyMgr = hotkeyMgr
	a.mouseMgr = mouseMgr
	a.hookMu.Unlock()
}

func (a *App) setContext(ctx context.Context) {
	a.ctxMu.Lock()
	a.ctx = ctx
	a.ctxMu.Unlock()
}

func (a *App) context() context.Context {
	a.ctxMu.RLock()
	defer a.ctxMu.RUnlock()
	return a.ctx
}

func (a *App) startup(ctx context.Context) {
	a.setContext(ctx)

	if err := a.configLoadErr; err != nil {
		log.Printf("[config] initial load error: %v", err)
		a.setStartupNotice(fmt.Sprintf("配置文件加载异常: %v", err))
		a.onConfigError(err)
	}

	initialCfg := a.GetConfig()
	if len(initialCfg.Groups) > 0 {
		a.SetActiveGroup(initialCfg.Groups[0].ID)
	}

	// Hooks exist before the watcher starts: a config reload arriving while the managers were
	// still nil used to drop the change without any trace.
	hotkeyMgr := hook.NewHotkeyManager(a.ToggleWindow)
	mouseMgr := hook.NewMouseHookManager(a.ToggleWindow)
	a.setHooks(hotkeyMgr, mouseMgr)

	a.pruneUsage(initialCfg)
	a.syncAutostart(initialCfg)

	if err := hotkeyMgr.Start(initialCfg.Settings.Hotkey); err != nil {
		log.Printf("[hook] failed to register hotkey %q: %v", initialCfg.Settings.Hotkey, err)
		status := HotkeyStatus{
			Hotkey:     initialCfg.Settings.Hotkey,
			Registered: false,
			Error:      err.Error(),
		}
		a.hookMu.Lock()
		a.appliedHotkey = ""
		a.hotkeyStatus = status
		a.hookMu.Unlock()
		a.emitEvent("hotkey:status", status)
		a.setStartupNotice(fmt.Sprintf("全局快捷键 %s 注册失败: %v", initialCfg.Settings.Hotkey, err))
	} else {
		status := HotkeyStatus{
			Hotkey:     initialCfg.Settings.Hotkey,
			Registered: true,
		}
		a.hookMu.Lock()
		a.appliedHotkey = initialCfg.Settings.Hotkey
		a.hotkeyStatus = status
		a.hookMu.Unlock()
		a.emitEvent("hotkey:status", status)
	}

	mouseMgr.SetSwallowInput(initialCfg.Settings.MouseSwallow)
	if err := mouseMgr.Update(initialCfg.Settings.MouseTrigger); err != nil {
		log.Printf("[hook] failed to set mouse trigger %q: %v", initialCfg.Settings.MouseTrigger, err)
	} else {
		a.hookMu.Lock()
		a.appliedMouse = initialCfg.Settings.MouseTrigger
		a.hookMu.Unlock()
	}

	// Start watching only now: the watcher may deliver settings that need both hook managers.
	if mgr := a.configManager(); mgr != nil {
		if err := mgr.StartWatching(); err != nil {
			log.Printf("[config] watch failed: %v", err)
		}
	}

	// File drops are handled by the frontend, which knows the selected group: it calls
	// AddDroppedFiles through the bridge from window.runtime.OnFileDrop. Subscribing here as well
	// would add every dropped file twice.
	tray.Start(tray.Callbacks{
		OnToggle: func() {
			a.ToggleWindow()
		},
		OnSettings: func() {
			a.ShowWindow()
			a.emitEvent("ui:open-settings", nil)
		},
		OnOpenConfig: func() {
			if err := a.OpenConfigFile(); err != nil {
				log.Printf("[tray] open config failed: %v", err)
			}
		},
		OnReload: func() {
			if err := a.ReloadConfig(); err != nil {
				log.Printf("[tray] reload config failed: %v", err)
			}
		},
		OnQuit: func() {
			a.Quit()
		},
	})
}

func (a *App) domReady(_ context.Context) {
	a.errMu.Lock()
	a.domReadyOK.Store(true)
	notice := a.startupNotice
	a.startupNotice = ""
	lastErr := a.lastConfigErr
	a.errMu.Unlock()

	if lastErr != "" {
		a.emitEvent("config:error", lastErr)
	}
	if notice != "" {
		a.emitToast("warning", notice)
	}
	a.emitEvent("hotkey:status", a.GetHotkeyStatus())

	// With start_hidden the window stays closed until the hotkey or the tray opens it, so it must
	// not be marked visible here or the blur monitor would treat it as an open window.
	if !a.GetConfig().Settings.StartHidden {
		a.stateMu.Lock()
		a.visible = true
		a.lastShown = time.Now()
		a.stateMu.Unlock()
		a.startBlurMonitor()
	}

	go func() {
		if a.findMainHwndFn != nil {
			if hwnd := a.findMainHwndFn(); hwnd != 0 {
				roundWindowCorners(hwnd)
				if a.focusWindowFn != nil {
					a.focusWindowFn(hwnd)
				}
			}
		}
	}()
}

func (a *App) beforeClose(_ context.Context) (prevent bool) {
	if a.quitting.Load() {
		return false
	}
	a.HideWindow()
	return true
}

func (a *App) shutdown(_ context.Context) {
	a.stopBlurMonitor()

	hotkeyMgr, mouseMgr := a.hooks()
	if hotkeyMgr != nil {
		hotkeyMgr.Stop()
	}
	if mouseMgr != nil {
		mouseMgr.Stop()
	}
	if mgr := a.configManager(); mgr != nil {
		_ = mgr.Close()
	}
	if a.usageStore != nil {
		_ = a.usageStore.Close()
	}
	a.flushWindowState()
	tray.Stop()
}

// Quit exits JeeDesk cleanly from the system tray.
func (a *App) Quit() {
	a.quitting.Store(true)
	if ctx := a.context(); ctx != nil {
		runtime.Quit(ctx)
	}
}

func (a *App) setStartupNotice(msg string) {
	if msg == "" {
		return
	}
	a.errMu.Lock()
	ready := a.domReadyOK.Load()
	if !ready {
		a.startupNotice = msg
	}
	a.errMu.Unlock()

	if ready {
		a.emitToast("warning", msg)
	}
}

func (a *App) emitEvent(name string, data ...interface{}) {
	ctx := a.context()
	if ctx == nil {
		return
	}
	runtime.EventsEmit(ctx, name, data...)
}

func (a *App) emitToast(level, message string) {
	if a.toastFunc != nil {
		a.toastFunc(level, message)
		return
	}
	a.emitEvent("ui:toast", toastPayload{
		Type:    level,
		Message: message,
	})
}

func (a *App) onConfigUpdated(cfg config.Config) {
	a.errMu.Lock()
	a.lastConfigErr = ""
	a.errMu.Unlock()

	a.syncHooks(cfg.Settings, false)
	a.ensureValidActiveGroup(cfg)
	a.pruneUsage(cfg)
	a.emitEvent("config:updated", cfg)
}

func (a *App) onConfigError(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	log.Printf("[config] reload error: %s", msg)
	a.emitEvent("config:error", msg)

	a.errMu.Lock()
	a.lastConfigErr = msg
	a.errMu.Unlock()
}

// applyHotkey switches the global hotkey to spec and records what the hook holds afterwards: a
// failed switch leaves the previous hotkey registered. explicit marks a change the user asked for.
// Otherwise a failure that has already been reported is not reported again, which keeps a config
// reload echoing a saved change from repeating the same message.
func (a *App) applyHotkey(spec string, explicit bool) {
	hotkeyMgr, _ := a.hooks()
	if hotkeyMgr == nil {
		return
	}
	err := hotkeyMgr.Update(spec)
	status := HotkeyStatus{Hotkey: spec}
	if err != nil {
		log.Printf("[hook] hotkey update failed (%q): %v", spec, err)
		status.Error = err.Error()
	} else {
		status.Registered = true
	}

	a.hookMu.Lock()
	prev := a.hotkeyStatus
	a.appliedHotkey = hotkeyMgr.ActiveSpec()
	a.hotkeyStatus = status
	a.hookMu.Unlock()
	a.emitEvent("hotkey:status", status)

	alreadyReported := !prev.Registered && prev.Hotkey == spec && prev.Error == status.Error
	if err != nil && (explicit || !alreadyReported) {
		a.emitToast("error", fmt.Sprintf("快捷键 %s 注册失败: %v", spec, err))
	}
}

// setHotkeyStatus replaces the reported hotkey state without touching the hook.
func (a *App) setHotkeyStatus(status HotkeyStatus) {
	a.hookMu.Lock()
	a.hotkeyStatus = status
	a.hookMu.Unlock()
	a.emitEvent("hotkey:status", status)
}

// syncHooks brings the global hotkey and mouse trigger in line with settings. explicit marks a
// change made in the app, as opposed to one picked up from config.json by the file watcher.
func (a *App) syncHooks(settings config.Settings, explicit bool) {
	a.hookMu.Lock()
	prevHotkey := a.appliedHotkey
	status := a.hotkeyStatus
	prevMouse := a.appliedMouse
	a.hookMu.Unlock()

	hotkeyMgr, mouseMgr := a.hooks()

	if hotkeyMgr != nil {
		switch {
		case settings.Hotkey != prevHotkey:
			a.applyHotkey(settings.Hotkey, explicit)
		case !status.Registered || status.Hotkey != settings.Hotkey:
			// The hook already holds the configured hotkey; only the reported status lags behind,
			// for example after a failed switch away from it.
			a.setHotkeyStatus(HotkeyStatus{Hotkey: settings.Hotkey, Registered: true})
		}
	}

	if mouseMgr == nil {
		return
	}
	mouseMgr.SetSwallowInput(settings.MouseSwallow)
	if settings.MouseTrigger != prevMouse {
		if err := mouseMgr.Update(settings.MouseTrigger); err != nil {
			log.Printf("[hook] mouse trigger update failed (%q): %v", settings.MouseTrigger, err)
			a.emitToast("error", fmt.Sprintf("鼠标呼出设置失败: %v", err))
		} else {
			a.hookMu.Lock()
			a.appliedMouse = settings.MouseTrigger
			a.hookMu.Unlock()
		}
	}
}

func (a *App) ensureValidActiveGroup(cfg config.Config) {
	current := a.getActiveGroupID()
	for _, g := range cfg.Groups {
		if g.ID == current {
			return
		}
	}
	if len(cfg.Groups) > 0 {
		a.SetActiveGroup(cfg.Groups[0].ID)
	} else {
		a.SetActiveGroup("")
	}
}

func (a *App) getActiveGroupID() string {
	a.activeGroupMu.RLock()
	defer a.activeGroupMu.RUnlock()
	return a.activeGroupID
}

// --- Window Visibility Methods ---

func (a *App) isCurrentlyVisible() bool {
	if a.findMainHwndFn != nil && a.isVisibleFn != nil {
		if hwnd := a.findMainHwndFn(); hwnd != 0 {
			return a.isVisibleFn(hwnd)
		}
	}
	a.stateMu.Lock()
	v := a.visible
	a.stateMu.Unlock()
	return v
}

func (a *App) focusMainWindow() {
	if a.findMainHwndFn == nil || a.focusWindowFn == nil {
		return
	}
	if hwnd := a.findMainHwndFn(); hwnd != 0 {
		a.focusWindowFn(hwnd)
	}
}

func (a *App) startBlurMonitor() {
	a.blurMu.Lock()
	defer a.blurMu.Unlock()

	if a.blurStopCh != nil {
		return
	}
	stopCh := make(chan struct{})
	a.blurStopCh = stopCh

	go func(ch <-chan struct{}) {
		ticker := time.NewTicker(blurMonitorInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ch:
				return
			case <-ticker.C:
				if a.shouldHideOnBlur() {
					a.HideWindow()
					return
				}
			}
		}
	}(stopCh)
}

func (a *App) stopBlurMonitor() {
	a.blurMu.Lock()
	ch := a.blurStopCh
	a.blurStopCh = nil
	a.blurMu.Unlock()

	if ch != nil {
		close(ch)
	}
}

func (a *App) shouldHideOnBlur() bool {
	if a.dialogOpen.Load() {
		return false
	}
	cfg := a.GetConfig()
	if !cfg.Settings.HideOnBlur {
		return false
	}

	a.stateMu.Lock()
	lastShown := a.lastShown
	a.stateMu.Unlock()

	if !a.isCurrentlyVisible() || time.Since(lastShown) < blurGracePeriod {
		return false
	}
	if a.isMouseDownFn != nil && a.isMouseDownFn() {
		return false
	}
	if a.isProcessFgFn != nil && a.isProcessFgFn() {
		return false
	}
	if a.findMainHwndFn != nil && a.isCursorInWinFn != nil {
		if hwnd := a.findMainHwndFn(); hwnd != 0 && a.isCursorInWinFn(hwnd) {
			return false
		}
	}
	return true
}

// ShowWindow displays JeeDesk and brings it to the foreground.
func (a *App) ShowWindow() {
	a.stateMu.Lock()
	a.visible = true
	a.lastShown = time.Now()
	a.stateMu.Unlock()

	ctx := a.context()
	if ctx == nil {
		return
	}
	runtime.WindowShow(ctx)
	a.startBlurMonitor()
	a.emitEvent("window:shown", nil)

	go func() {
		a.focusMainWindow()
		time.Sleep(40 * time.Millisecond)
		a.stateMu.Lock()
		stillVisible := a.visible
		a.stateMu.Unlock()
		if stillVisible {
			a.focusMainWindow()
		}
	}()
}

// HideWindow hides JeeDesk back to the system tray.
func (a *App) HideWindow() {
	a.stopBlurMonitor()

	a.stateMu.Lock()
	a.visible = false
	a.stateMu.Unlock()

	ctx := a.context()
	if ctx == nil {
		return
	}
	runtime.WindowHide(ctx)
}

// ToggleWindow toggles JeeDesk window visibility, bringing an unfocused
// visible window to the foreground instead of hiding it.
func (a *App) ToggleWindow() {
	if !a.isCurrentlyVisible() {
		a.ShowWindow()
		return
	}
	if a.isProcessFgFn != nil && !a.isProcessFgFn() {
		a.ShowWindow()
		return
	}
	a.HideWindow()
}

// OnWindowBlur is invoked by the frontend when the window loses focus.
func (a *App) OnWindowBlur() {
	if a.shouldHideOnBlur() {
		a.HideWindow()
	}
}

// --- Wails Bound Methods ---

// GetHotkeyStatus returns the current global hotkey registration status.
func (a *App) GetHotkeyStatus() HotkeyStatus {
	a.hookMu.Lock()
	defer a.hookMu.Unlock()
	return a.hotkeyStatus
}

// GetConfig returns the current valid configuration.
func (a *App) GetConfig() config.Config {
	if a.configManager() == nil {
		return config.DefaultConfig()
	}
	return a.configManager().Get()
}

// SetActiveGroup tracks the currently selected group ID for native file drops.
func (a *App) SetActiveGroup(groupID string) {
	a.activeGroupMu.Lock()
	a.activeGroupID = strings.TrimSpace(groupID)
	a.activeGroupMu.Unlock()
}

// LaunchItem launches the specified item using Win32 ShellExecuteW.
func (a *App) LaunchItem(groupID string, itemID string) error {
	cfg := a.GetConfig()
	item, found := findItem(cfg, groupID, itemID)
	if !found {
		return fmt.Errorf("未找到目标项目 (%s/%s)", groupID, itemID)
	}
	launchErr := launcher.Launch(item.Path, item.Args, item.WorkDir)
	if item.Admin {
		launchErr = launcher.LaunchElevated(item.Path, item.Args, item.WorkDir)
	}
	if launchErr != nil {
		// A cancelled UAC prompt is not a failure worth a toast, but it must not count as a
		// launch either, otherwise the recent list fills with items the user never started.
		return launchErr
	}

	a.recordLaunch(groupID, item.ID)
	if cfg.Settings.HideOnLaunch {
		a.HideWindow()
	}
	return nil
}

// AddDroppedFiles resolves dropped file/folder/.lnk paths, extracts & caches icons,
// and appends the new items into the specified group via config.json.
func (a *App) AddDroppedFiles(groupID string, paths []string) error {
	if a.configManager() == nil {
		return errors.New("配置管理器尚未就绪")
	}
	if len(paths) == 0 {
		return nil
	}

	resolvedItems := make([]config.Item, 0, len(paths))
	for _, rawPath := range paths {
		trimmed := strings.TrimSpace(rawPath)
		if trimmed == "" {
			continue
		}

		target, err := resolver.ResolvePath(trimmed)
		if err != nil {
			log.Printf("[resolver] resolve %q failed: %v", trimmed, err)
			continue
		}

		iconSrc := target.IconSrc
		if iconSrc == "" {
			iconSrc = target.Path
		}
		iconRel, iconErr := resolver.ExtractAndCacheIcon(iconSrc, a.iconsDir)
		if iconErr != nil && iconSrc != target.Path {
			iconRel, iconErr = resolver.ExtractAndCacheIcon(target.Path, a.iconsDir)
		}
		if iconErr != nil {
			log.Printf("[resolver] icon extract failed for %q: %v", target.Path, iconErr)
			iconRel = ""
		}

		resolvedItems = append(resolvedItems, config.Item{
			ID:      generateID("item"),
			Name:    target.Name,
			Path:    target.Path,
			Args:    target.Args,
			WorkDir: target.WorkDir,
			Icon:    iconRel,
		})
	}

	if len(resolvedItems) == 0 {
		return errors.New("未能解析任何有效文件")
	}

	err := a.configManager().Update(func(cfg *config.Config) error {
		if len(cfg.Groups) == 0 {
			cfg.Groups = append(cfg.Groups, config.Group{
				ID:    "default",
				Name:  "常用工具",
				Items: []config.Item{},
			})
		}
		idx := findGroupIndex(cfg.Groups, groupID)
		if idx < 0 {
			idx = 0
		}
		cfg.Groups[idx].Items = append(cfg.Groups[idx].Items, resolvedItems...)
		return nil
	})
	if err != nil {
		return err
	}

	if len(resolvedItems) == 1 {
		a.emitToast("success", fmt.Sprintf("已添加「%s」", resolvedItems[0].Name))
	} else {
		a.emitToast("success", fmt.Sprintf("已添加 %d 个项目", len(resolvedItems)))
	}
	return nil
}

// AddItem manually adds a single item to groupID, auto-extracting an icon if empty.
func (a *App) AddItem(groupID string, item config.Item) error {
	if a.configManager() == nil {
		return errors.New("配置管理器尚未就绪")
	}
	item.Name = strings.TrimSpace(item.Name)
	item.Path = strings.TrimSpace(item.Path)
	item.Args = strings.TrimSpace(item.Args)
	item.WorkDir = strings.TrimSpace(item.WorkDir)
	item.Icon = strings.TrimSpace(item.Icon)

	if item.Path == "" {
		return errors.New("目标路径不能为空")
	}
	if strings.EqualFold(filepath.Ext(item.Path), ".lnk") {
		if resolved, err := resolver.ResolvePath(item.Path); err == nil {
			item.Path = resolved.Path
			if item.Name == "" {
				item.Name = resolved.Name
			}
			if item.Args == "" {
				item.Args = resolved.Args
			}
			if item.WorkDir == "" {
				item.WorkDir = resolved.WorkDir
			}
		}
	}
	if item.Name == "" {
		item.Name = inferItemName(item.Path)
	}
	if item.ID == "" {
		item.ID = generateID("item")
	}
	if item.Icon == "" {
		if rel, err := resolver.ExtractAndCacheIcon(item.Path, a.iconsDir); err == nil {
			item.Icon = rel
		}
	}

	return a.configManager().Update(func(cfg *config.Config) error {
		idx := findGroupIndex(cfg.Groups, groupID)
		if idx < 0 {
			return fmt.Errorf("未找到分组 %q", groupID)
		}
		cfg.Groups[idx].Items = append(cfg.Groups[idx].Items, item)
		return nil
	})
}

// UpdateItem updates an existing item's metadata inside groupID.
func (a *App) UpdateItem(groupID string, item config.Item) error {
	if a.configManager() == nil {
		return errors.New("配置管理器尚未就绪")
	}
	item.Name = strings.TrimSpace(item.Name)
	item.Path = strings.TrimSpace(item.Path)
	item.Args = strings.TrimSpace(item.Args)
	item.WorkDir = strings.TrimSpace(item.WorkDir)
	item.Icon = strings.TrimSpace(item.Icon)

	if item.ID == "" {
		return errors.New("项目 ID 不能为空")
	}
	if item.Path == "" {
		return errors.New("目标路径不能为空")
	}
	if item.Name == "" {
		item.Name = inferItemName(item.Path)
	}
	if item.Icon == "" {
		if rel, err := resolver.ExtractAndCacheIcon(item.Path, a.iconsDir); err == nil {
			item.Icon = rel
		}
	}

	return a.configManager().Update(func(cfg *config.Config) error {
		gIdx := findGroupIndex(cfg.Groups, groupID)
		if gIdx < 0 {
			return fmt.Errorf("未找到分组 %q", groupID)
		}
		for i := range cfg.Groups[gIdx].Items {
			if cfg.Groups[gIdx].Items[i].ID == item.ID {
				cfg.Groups[gIdx].Items[i] = item
				return nil
			}
		}
		return fmt.Errorf("未找到项目 %q", item.ID)
	})
}

// DeleteItem removes an item from groupID. The removed item is remembered for UndoDeleteItem.
func (a *App) DeleteItem(groupID string, itemID string) error {
	if a.configManager() == nil {
		return errors.New("配置管理器尚未就绪")
	}

	snapshot := a.itemSnapshot(groupID, itemID)

	err := a.configManager().Update(func(cfg *config.Config) error {
		gIdx := findGroupIndex(cfg.Groups, groupID)
		if gIdx < 0 {
			return fmt.Errorf("未找到分组 %q", groupID)
		}
		items := cfg.Groups[gIdx].Items
		for i := range items {
			if items[i].ID == itemID {
				cfg.Groups[gIdx].Items = append(items[:i], items[i+1:]...)
				return nil
			}
		}
		return fmt.Errorf("未找到项目 %q", itemID)
	})
	if err != nil {
		return err
	}

	if a.usageStore != nil {
		a.usageStore.Forget(groupID, itemID)
	}
	if snapshot != nil {
		a.undoMu.Lock()
		a.deletedItem = snapshot
		a.deletedGroup = nil
		a.undoMu.Unlock()
	}
	return nil
}

// OpenItemLocation opens Windows Explorer and selects the target path.
func (a *App) OpenItemLocation(path string) error {
	return launcher.OpenFolderAndSelect(path)
}

// AddGroup appends a new group to config.json, makes it the active group, and returns its ID.
func (a *App) AddGroup(name string) (string, error) {
	if a.configManager() == nil {
		return "", errors.New("配置管理器尚未就绪")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("分组名称不能为空")
	}

	newGroup := config.Group{
		ID:    generateID("grp"),
		Name:  trimmed,
		Items: []config.Item{},
	}
	if err := a.configManager().Update(func(cfg *config.Config) error {
		cfg.Groups = append(cfg.Groups, newGroup)
		return nil
	}); err != nil {
		return "", err
	}
	a.SetActiveGroup(newGroup.ID)
	return newGroup.ID, nil
}

// RenameGroup renames an existing group.
func (a *App) RenameGroup(groupID string, name string) error {
	if a.configManager() == nil {
		return errors.New("配置管理器尚未就绪")
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("分组名称不能为空")
	}

	return a.configManager().Update(func(cfg *config.Config) error {
		idx := findGroupIndex(cfg.Groups, groupID)
		if idx < 0 {
			return fmt.Errorf("未找到分组 %q", groupID)
		}
		cfg.Groups[idx].Name = trimmed
		return nil
	})
}

// DeleteGroup deletes a group by ID. The group and its items are remembered for UndoDeleteGroup.
func (a *App) DeleteGroup(groupID string) error {
	if a.configManager() == nil {
		return errors.New("配置管理器尚未就绪")
	}

	snapshot := a.groupSnapshot(groupID)

	err := a.configManager().Update(func(cfg *config.Config) error {
		idx := findGroupIndex(cfg.Groups, groupID)
		if idx < 0 {
			return fmt.Errorf("未找到分组 %q", groupID)
		}
		cfg.Groups = append(cfg.Groups[:idx], cfg.Groups[idx+1:]...)
		return nil
	})
	if err != nil {
		return err
	}
	if a.usageStore != nil {
		a.usageStore.ForgetGroup(groupID)
	}
	if snapshot != nil {
		a.undoMu.Lock()
		a.deletedGroup = snapshot
		a.deletedItem = nil
		a.undoMu.Unlock()
	}
	return nil
}

// SaveSettings validates and writes updated application settings to config.json.
func (a *App) SaveSettings(settings config.Settings) error {
	if a.configManager() == nil {
		return errors.New("配置管理器尚未就绪")
	}

	settings.Hotkey = strings.TrimSpace(settings.Hotkey)
	if settings.Hotkey == "" {
		settings.Hotkey = config.DefaultHotkey
	}
	if _, _, err := hook.ParseHotkey(settings.Hotkey); err != nil {
		return fmt.Errorf("无效快捷键 %q: %w", settings.Hotkey, err)
	}

	// Validate what the user typed before defaulting, so a typo is reported instead of being
	// silently replaced by a default.
	mouseTrigger, err := hook.ValidateMouseTrigger(settings.MouseTrigger)
	if err != nil {
		return fmt.Errorf("无效鼠标触发设置: %w", err)
	}
	settings.MouseTrigger = mouseTrigger

	// Fill in and clamp every remaining field, including ones an older client does not send.
	normalized := config.DefaultConfig()
	normalized.Settings = settings
	config.Normalize(&normalized)
	settings = normalized.Settings

	if err := a.configManager().Update(func(cfg *config.Config) error {
		cfg.Settings = settings
		return nil
	}); err != nil {
		return err
	}

	a.syncHooks(settings, true)
	if err := applyAutoStart(settings.AutoStart); err != nil {
		a.emitToast("warning", fmt.Sprintf("开机启动设置失败: %v", err))
	}
	a.emitToast("success", "设置已保存")
	return nil
}

// ReloadConfig forces an immediate reload of config.json. It is also how the user retries a hotkey
// that failed to register, for example after the program holding it was closed.
func (a *App) ReloadConfig() error {
	if mgr := a.configManager(); mgr == nil {
		rebuilt, err := config.NewManager(a.configPath, a.onConfigUpdated, a.onConfigError)
		if err != nil {
			a.onConfigError(err)
			return err
		}
		a.setConfigManager(rebuilt)
		_ = rebuilt.StartWatching()
		cfg := rebuilt.Get()
		a.onConfigUpdated(cfg)
		// The user asked for this reload, so a hotkey that still fails to register is reported.
		a.syncHooks(cfg.Settings, true)
		a.emitToast("success", "配置已重新加载")
		return nil
	}

	cfg, err := a.configManager().Reload()
	if err != nil {
		return err
	}
	a.syncHooks(cfg.Settings, true)
	a.emitToast("success", "配置已重新加载")
	return nil
}

// OpenConfigFile opens config.json in the user's default editor.
func (a *App) OpenConfigFile() error {
	return launcher.OpenFile(a.configPath)
}

// PickFile opens a native Windows file chooser while suppressing hide_on_blur.
func (a *App) PickFile(title string, filterName string, pattern string) (string, error) {
	ctx := a.context()
	if ctx == nil {
		return "", errors.New("应用上下文未初始化")
	}

	a.dialogOpen.Store(true)
	defer func() {
		a.stateMu.Lock()
		a.lastShown = time.Now()
		a.stateMu.Unlock()
		a.dialogOpen.Store(false)
	}()

	opts := runtime.OpenDialogOptions{
		Title: title,
	}
	if strings.TrimSpace(pattern) != "" {
		name := strings.TrimSpace(filterName)
		if name == "" {
			name = "Files"
		}
		opts.Filters = []runtime.FileFilter{
			{DisplayName: name, Pattern: pattern},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		}
	}

	return runtime.OpenFileDialog(ctx, opts)
}

// PickDirectory opens a native Windows folder chooser while suppressing hide_on_blur.
func (a *App) PickDirectory(title string) (string, error) {
	ctx := a.context()
	if ctx == nil {
		return "", errors.New("应用上下文未初始化")
	}

	a.dialogOpen.Store(true)
	defer func() {
		a.stateMu.Lock()
		a.lastShown = time.Now()
		a.stateMu.Unlock()
		a.dialogOpen.Store(false)
	}()

	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title: title,
	})
}

// --- Helpers ---

func findGroupIndex(groups []config.Group, groupID string) int {
	for i := range groups {
		if groups[i].ID == groupID {
			return i
		}
	}
	return -1
}

func findItem(cfg config.Config, groupID, itemID string) (config.Item, bool) {
	gIdx := findGroupIndex(cfg.Groups, groupID)
	if gIdx < 0 {
		return config.Item{}, false
	}
	for _, it := range cfg.Groups[gIdx].Items {
		if it.ID == itemID {
			return it, true
		}
	}
	return config.Item{}, false
}

func generateID(prefix string) string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b[:]))
}

func inferItemName(path string) string {
	cleaned := strings.TrimRight(strings.TrimSpace(path), `\/`)
	base := filepath.Base(cleaned)
	ext := filepath.Ext(base)
	if ext != "" && len(ext) < len(base) {
		return strings.TrimSuffix(base, ext)
	}
	if base == "" || base == "." {
		return path
	}
	return base
}
