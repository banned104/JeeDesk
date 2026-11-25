package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
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
	recentItemsLimit    = 12
)

// appVersion is reported in the about section and compared against the latest release. Release
// builds override it with -ldflags "-X main.appVersion=<version>".
var appVersion = "1.0.0"

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

	// startHidden records whether this launch should stay in the tray. It covers both
	// settings.start_hidden and --silent, and is written once before wails.Run starts.
	startHidden bool

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

	iconFillMu    sync.Mutex
	iconFillTimer *time.Timer

	activeGroupMu sync.RWMutex
	activeGroupID string

	stateMu   sync.Mutex
	visible   bool
	lastShown time.Time
	// previousRunCrashed is set by main when the marker from the last run was still on disk.
	previousRunCrashed bool
	// nativeDialogOpen covers Win32 pickers, uiModalOpen covers frontend dialogs. They are kept
	// apart because closing a picker must not clear a modal that is still on screen.
	nativeDialogOpen atomic.Bool
	uiModalOpen      atomic.Bool
	// pendingShow remembers a wake request that arrived before the Wails context existed.
	pendingShow atomic.Bool
	quitting    atomic.Bool
	domReadyOK  atomic.Bool

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

	if a.pendingShow.Swap(false) {
		a.ShowWindow()
	}

	// Start watching only now: the watcher may deliver settings that need both hook managers.
	if mgr := a.configManager(); mgr != nil {
		// Losing config.json is not an edit the UI can render; it needs a banner so deleting the
		// file by accident is visible instead of silently reverting to the in-memory copy.
		mgr.SetRemovedHandler(func() {
			a.emitEvent("config:removed", nil)
			a.emitToast("warning", "config.json 已被删除，当前仍在用内存中的配置，可在设置中重建")
		})
		if err := mgr.StartWatching(); err != nil {
			log.Printf("[config] watch failed: %v", err)
		}
	}

	// Missing icons are filled in and orphans are removed in the background: both touch the disk
	// and must not delay the first paint. Filling runs first so cleanup cannot drop a fresh file.
	go func() {
		// Cached PNGs are keyed by target path, so a better extractor has to invalidate them once.
		if err := a.syncIconCacheVersion(); err != nil {
			log.Printf("[icons] cache version sync failed: %v", err)
		}
		if filled, err := a.EnsureIcons(); err != nil {
			log.Printf("[icons] backfill failed: %v", err)
		} else if filled > 0 {
			log.Printf("[icons] backfilled %d icons", filled)
		}
		if _, err := a.CleanupIcons(); err != nil {
			log.Printf("[icons] cleanup failed: %v", err)
		}
	}()

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
		OnOpenBaseDir: func() {
			if err := a.OpenBaseDir(); err != nil {
				log.Printf("[tray] open base dir failed: %v", err)
			}
		},
		OnOpenLog: func() {
			if err := a.OpenLogFile(); err != nil {
				log.Printf("[tray] open log failed: %v", err)
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
	if a.startHidden {
		go a.hideAfterWebviewShow()
	} else {
		a.stateMu.Lock()
		a.visible = true
		a.lastShown = time.Now()
		a.stateMu.Unlock()
		a.startBlurMonitor()
	}

	if a.previousRunCrashed {
		a.previousRunCrashed = false
		a.emitToast("warning", "上次没有正常退出，日志与现场见 logs 目录")
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

	a.iconFillMu.Lock()
	if a.iconFillTimer != nil {
		a.iconFillTimer.Stop()
		a.iconFillTimer = nil
	}
	a.iconFillMu.Unlock()

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
	ctx := a.context()
	if ctx == nil {
		// Before OnStartup there is nothing to quit through Wails; exiting the process is the only
		// way left, and a tray "quit" that did nothing would be worse than a hard exit.
		log.Printf("[app] quit requested before the runtime context exists; exiting")
		os.Exit(0)
	}
	runtime.Quit(ctx)
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
	a.scheduleIconFill(cfg)
	if len(cfg.Warnings) > 0 {
		a.emitToast("warning", strings.Join(cfg.Warnings, "；"))
	}
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

// scheduleIconFill extracts icons for items that arrived without one, such as an item hand-written
// into config.json. It is debounced because a config update arrives in bursts while dragging.
func (a *App) scheduleIconFill(cfg config.Config) {
	needsFill := false
	for _, group := range cfg.Groups {
		for _, item := range group.Items {
			if strings.TrimSpace(item.Icon) == "" && targetExists(item.Path) {
				needsFill = true
				break
			}
		}
		if needsFill {
			break
		}
	}
	if !needsFill {
		return
	}

	a.iconFillMu.Lock()
	defer a.iconFillMu.Unlock()
	if a.iconFillTimer != nil {
		a.iconFillTimer.Stop()
	}
	a.iconFillTimer = time.AfterFunc(1200*time.Millisecond, func() {
		if _, err := a.EnsureIcons(); err != nil {
			log.Printf("[icons] background fill failed: %v", err)
		}
	})
}

// applyHotkey switches the global hotkey to spec and records what the hook holds afterwards: a
// failed switch leaves the previous hotkey registered. explicit marks a change the user asked for.
// Otherwise a failure that has already been reported is not reported again, which keeps a config
// reload echoing a saved change from repeating the same message.
func (a *App) applyHotkey(spec string, explicit bool) HotkeyStatus {
	hotkeyMgr, _ := a.hooks()
	if hotkeyMgr == nil {
		return HotkeyStatus{Hotkey: spec, Error: "快捷键管理器尚未就绪"}
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
	return status
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
	if a.modalOpen() {
		return false
	}
	mgr := a.configManager()
	if mgr == nil {
		return false
	}
	settings := mgr.Settings()
	if !settings.HideOnBlur {
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
	// Leaving the cursor on the panel used to count as "still inside", so the window survived a
	// focus switch. Strict blur drops that exemption for users who want the launcher gone the
	// moment another application takes over.
	if !settings.StrictBlur && a.findMainHwndFn != nil && a.isCursorInWinFn != nil {
		if hwnd := a.findMainHwndFn(); hwnd != 0 && a.isCursorInWinFn(hwnd) {
			return false
		}
	}
	return true
}

// modalOpen reports whether a dialog of any kind is on screen.
func (a *App) modalOpen() bool {
	return a.nativeDialogOpen.Load() || a.uiModalOpen.Load()
}

// hideAfterWebviewShow hides the panel once Wails has finished its own "make the webview visible"
// call. Wails runs OnDomReady before that call, so an immediate WindowHide would be undone; the
// delayed hides stop as soon as the panel counts as shown, so a fast hotkey press wins.
func (a *App) hideAfterWebviewShow() {
	for _, delay := range []time.Duration{150 * time.Millisecond, 700 * time.Millisecond} {
		time.Sleep(delay)
		// The panel counts as shown only after ShowWindow or ToggleWindow ran; the raw Win32
		// visibility is still true here because Wails just showed the webview itself.
		a.stateMu.Lock()
		shown := a.visible
		a.stateMu.Unlock()
		if shown {
			return
		}
		ctx := a.context()
		if ctx == nil {
			return
		}
		runtime.WindowHide(ctx)
	}
}

// ShowWindow displays JeeDesk and brings it to the foreground.
func (a *App) ShowWindow() {
	// Focus stealing is only needed when the window was hidden. Showing a window that is already
	// up (for example on a hotkey press while it has focus) would grab it three times in a row.
	wasVisible := a.isCurrentlyVisible()

	a.stateMu.Lock()
	a.visible = true
	a.lastShown = time.Now()
	a.stateMu.Unlock()

	ctx := a.context()
	if ctx == nil {
		// A second-instance wake can arrive before OnStartup installed the context; remember the
		// request so startup can honour it instead of dropping it.
		a.pendingShow.Store(true)
		return
	}
	runtime.WindowShow(ctx)
	a.startBlurMonitor()
	a.emitEvent("window:shown", nil)
	// Windows silently drops low-level hooks whose callback timed out while the panel was hidden;
	// asking the manager for the current trigger rebuilds a dead hook and is a no-op otherwise.
	a.refreshMouseHook()

	if wasVisible {
		return
	}
	go func() {
		time.Sleep(30 * time.Millisecond)
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
	a.emitEvent("window:hidden", nil)
}

// refreshMouseHook re-applies the configured mouse trigger, which reinstalls a hook Windows dropped.
func (a *App) refreshMouseHook() {
	_, mouseMgr := a.hooks()
	if mouseMgr == nil {
		return
	}
	trigger := a.GetConfig().Settings.MouseTrigger
	if err := mouseMgr.Update(trigger); err != nil {
		log.Printf("[hook] failed to re-apply mouse trigger %q on show: %v", trigger, err)
	}
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
	var launchErr error
	if item.Admin {
		launchErr = launcher.LaunchElevated(item.Path, item.Args, item.WorkDir)
	} else {
		launchErr = launcher.Launch(item.Path, item.Args, item.WorkDir)
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
	failed := make([]string, 0)
	for _, rawPath := range paths {
		trimmed := strings.TrimSpace(rawPath)
		if trimmed == "" {
			continue
		}

		target, err := resolver.ResolvePath(trimmed)
		if err != nil {
			log.Printf("[resolver] resolve %q failed: %v", trimmed, err)
			failed = append(failed, filepath.Base(trimmed))
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
			// Silently falling back to the first group put files somewhere the user never
			// looked. Dropping onto a group that just disappeared is an error instead.
			return fmt.Errorf("未找到分组 %q", groupID)
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
	if len(failed) > 0 {
		a.emitToast("warning", fmt.Sprintf("%d 个文件无法解析: %s", len(failed), strings.Join(failed, "、")))
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

	// A cached icon belongs to the old target: keeping it would show the previous program's icon
	// after the path was corrected.
	previousPath := ""
	if old, found := findItem(a.GetConfig(), groupID, item.ID); found {
		previousPath = old.Path
	}
	pathChanged := previousPath != "" && !strings.EqualFold(previousPath, item.Path)
	if pathChanged && strings.HasPrefix(item.Icon, "icons/") {
		item.Icon = ""
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

	previous := a.GetConfig().Settings
	// Register first, write second: a hotkey that cannot be registered must not end up in
	// config.json, or every later start would fail to install it too. Before OnStartup there is no
	// hook manager yet, and saving must still work.
	if hotkeyMgr, _ := a.hooks(); hotkeyMgr != nil {
		hookStatus := a.applyHotkey(settings.Hotkey, true)
		if hookStatus.Error != "" {
			// The old hotkey is still registered, so the file must keep saying so. Returning the
			// error keeps the settings dialog open with the reason instead of saving a hotkey
			// that does not work.
			a.syncHooks(previous, true)
			return fmt.Errorf("快捷键 %s 无法注册: %s", settings.Hotkey, hookStatus.Error)
		}
	}

	// Window geometry is owned by SaveWindowState, which writes quietly and never reaches the
	// frontend. A settings save carrying the geometry from startup would move the panel back.
	geometry := previous
	settings.WindowX, settings.WindowY = geometry.WindowX, geometry.WindowY
	settings.WindowWidth, settings.WindowHeight = geometry.WindowWidth, geometry.WindowHeight
	settings.WindowPositioned = geometry.WindowPositioned

	if err := a.configManager().Update(func(cfg *config.Config) error {
		cfg.Settings = settings
		return nil
	}); err != nil {
		a.syncHooks(previous, true)
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
	mgr := a.configManager()
	if mgr == nil {
		return errors.New("配置管理器尚未就绪")
	}
	// Force: the user asked for this reload, even if the file did not change on disk.
	cfg, err := mgr.ReloadForce()
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

	a.nativeDialogOpen.Store(true)
	defer func() {
		a.stateMu.Lock()
		a.lastShown = time.Now()
		a.stateMu.Unlock()
		a.nativeDialogOpen.Store(false)
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

	a.nativeDialogOpen.Store(true)
	defer func() {
		a.stateMu.Lock()
		a.lastShown = time.Now()
		a.stateMu.Unlock()
		a.nativeDialogOpen.Store(false)
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
	return resolver.DisplayName(path)
}
