package main

import (
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"jeedesk/internal/applog"
	"jeedesk/internal/autostart"
	"jeedesk/internal/config"
	"jeedesk/internal/launcher"
	"jeedesk/internal/resolver"
	"jeedesk/internal/search"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	searchResultsLimit = 60
	maxImportCount     = 500
)

// deletedItemSnapshot remembers a deleted item so Ctrl+Z can put it back where it was.
type deletedItemSnapshot struct {
	groupID string
	index   int
	item    config.Item
}

// deletedGroupSnapshot remembers a deleted group including all of its items.
type deletedGroupSnapshot struct {
	index int
	group config.Group
}

// SearchHit is one search result. Fields are flat so the frontend can render a row without
// resolving the item inside its group first.
type SearchHit struct {
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name"`
	ItemID    string `json:"item_id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Args      string `json:"args"`
	Icon      string `json:"icon"`
	Admin     bool   `json:"admin"`
	Score     int    `json:"score"`
}

// RecentItem is one entry of the "recently used" list.
type RecentItem struct {
	GroupID   string `json:"group_id"`
	GroupName string `json:"group_name"`
	ItemID    string `json:"item_id"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Icon      string `json:"icon"`
	Admin     bool   `json:"admin"`
	Count     int    `json:"count"`
	LastUsed  int64  `json:"last_used"`
}

// AppInfo describes the running instance, paths, and the registry state of autostart.
type AppInfo struct {
	Version        string `json:"version"`
	BaseDir        string `json:"base_dir"`
	ConfigPath     string `json:"config_path"`
	ConfigBackup   string `json:"config_backup"`
	IconsDir       string `json:"icons_dir"`
	LogPath        string `json:"log_path"`
	ExePath        string `json:"exe_path"`
	AutoStart      bool   `json:"auto_start"`
	AutoStartError string `json:"auto_start_error"`
}

// SearchItems searches every group by name and target path. Results are ordered by match quality,
// then by how often the item was launched, so a frequently used item wins a tie.
func (a *App) SearchItems(query string) []SearchHit {
	tokens := search.Query(query)
	if len(tokens) == 0 {
		return []SearchHit{}
	}

	cfg := a.GetConfig()
	hits := make([]SearchHit, 0, 32)
	for _, group := range cfg.Groups {
		for _, item := range group.Items {
			match, ok := search.Match(tokens, item.Name, item.Path)
			if !ok {
				continue
			}
			hits = append(hits, SearchHit{
				GroupID:   group.ID,
				GroupName: group.Name,
				ItemID:    item.ID,
				Name:      item.Name,
				Path:      item.Path,
				Args:      item.Args,
				Icon:      item.Icon,
				Admin:     item.Admin,
				Score:     match.Score,
			})
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		left := a.usageCount(hits[i].GroupID, hits[i].ItemID)
		right := a.usageCount(hits[j].GroupID, hits[j].ItemID)
		if left != right {
			return left > right
		}
		return strings.ToLower(hits[i].Name) < strings.ToLower(hits[j].Name)
	})

	if len(hits) > searchResultsLimit {
		hits = hits[:searchResultsLimit]
	}
	return hits
}

// GetRecentItems returns recently launched items, most recent first, skipping items that have
// since been deleted.
func (a *App) GetRecentItems(limit int) []RecentItem {
	if a.usageStore == nil {
		return []RecentItem{}
	}
	if limit <= 0 || limit > recentItemsLimit {
		limit = recentItemsLimit
	}

	cfg := a.GetConfig()
	out := make([]RecentItem, 0, limit)
	for _, entry := range a.usageStore.Snapshot() {
		if len(out) >= limit {
			break
		}
		groupID, group, item, found := lookupItem(cfg, entry.GroupID, entry.ItemID)
		if !found {
			continue
		}
		out = append(out, RecentItem{
			GroupID:   groupID,
			GroupName: group.Name,
			ItemID:    item.ID,
			Name:      item.Name,
			Path:      item.Path,
			Icon:      item.Icon,
			Admin:     item.Admin,
			Count:     entry.Count,
			LastUsed:  entry.LastUsed,
		})
	}
	return out
}

// ClearRecentItems forgets the launch history.
func (a *App) ClearRecentItems() error {
	if a.usageStore == nil {
		return errors.New("使用记录不可用")
	}
	for _, entry := range a.usageStore.Snapshot() {
		a.usageStore.Forget(entry.GroupID, entry.ItemID)
	}
	if err := a.usageStore.Flush(); err != nil {
		return err
	}
	a.emitEvent("usage:updated", nil)
	return nil
}

func (a *App) usageCount(groupID, itemID string) int {
	if a.usageStore == nil {
		return 0
	}
	return a.usageStore.Count(groupID, itemID)
}

// recordLaunch stores the launch and refreshes the recent list.
func (a *App) recordLaunch(groupID, itemID string) {
	if a.usageStore == nil {
		return
	}
	a.usageStore.Record(groupID, itemID)
	a.emitEvent("usage:updated", nil)
}

// pruneUsage drops history of items that no longer exist.
func (a *App) pruneUsage(cfg config.Config) {
	if a.usageStore == nil {
		return
	}
	keep := make(map[string]map[string]bool, len(cfg.Groups))
	for _, group := range cfg.Groups {
		items := make(map[string]bool, len(group.Items))
		for _, item := range group.Items {
			items[item.ID] = true
		}
		keep[group.ID] = items
	}
	a.usageStore.Prune(keep)
}

// SetItemAdmin flips only the admin flag of an item. The frontend often holds a partial item (from
// a search hit or the recent list); replacing the whole item with it would wipe the arguments and
// working directory the user had saved.
func (a *App) SetItemAdmin(groupID string, itemID string, admin bool) error {
	mgr := a.configManager()
	if mgr == nil {
		return errors.New("配置管理器尚未就绪")
	}
	return mgr.Update(func(cfg *config.Config) error {
		gIdx := findGroupIndex(cfg.Groups, groupID)
		if gIdx < 0 {
			return fmt.Errorf("未找到分组 %q", groupID)
		}
		for i := range cfg.Groups[gIdx].Items {
			if cfg.Groups[gIdx].Items[i].ID == itemID {
				cfg.Groups[gIdx].Items[i].Admin = admin
				return nil
			}
		}
		return fmt.Errorf("未找到项目 %q", itemID)
	})
}

// MoveItem moves an item to another group and position, or reorders it inside its own group.
func (a *App) MoveItem(fromGroupID string, itemID string, toGroupID string, toIndex int) error {
	mgr := a.configManager()
	if mgr == nil {
		return errors.New("配置管理器尚未就绪")
	}

	return mgr.Update(func(cfg *config.Config) error {
		fromIdx := findGroupIndex(cfg.Groups, fromGroupID)
		if fromIdx < 0 {
			return fmt.Errorf("未找到分组 %q", fromGroupID)
		}
		toIdx := findGroupIndex(cfg.Groups, toGroupID)
		if toIdx < 0 {
			// Dropping onto a group that just disappeared would lose the item; refuse instead.
			return fmt.Errorf("未找到目标分组 %q", toGroupID)
		}

		itemPos := -1
		for i := range cfg.Groups[fromIdx].Items {
			if cfg.Groups[fromIdx].Items[i].ID == itemID {
				itemPos = i
				break
			}
		}
		if itemPos < 0 {
			return fmt.Errorf("未找到项目 %q", itemID)
		}

		item := cfg.Groups[fromIdx].Items[itemPos]
		cfg.Groups[fromIdx].Items = append(cfg.Groups[fromIdx].Items[:itemPos], cfg.Groups[fromIdx].Items[itemPos+1:]...)

		items := cfg.Groups[toIdx].Items
		if toIndex < 0 {
			toIndex = 0
		}
		if toIndex > len(items) {
			toIndex = len(items)
		}
		items = append(items, config.Item{})
		copy(items[toIndex+1:], items[toIndex:])
		items[toIndex] = item
		cfg.Groups[toIdx].Items = items
		return nil
	})
}

// MoveGroup reorders a group inside the sidebar.
func (a *App) MoveGroup(groupID string, toIndex int) error {
	mgr := a.configManager()
	if mgr == nil {
		return errors.New("配置管理器尚未就绪")
	}

	return mgr.Update(func(cfg *config.Config) error {
		idx := findGroupIndex(cfg.Groups, groupID)
		if idx < 0 {
			return fmt.Errorf("未找到分组 %q", groupID)
		}
		group := cfg.Groups[idx]
		rest := append(cfg.Groups[:idx], cfg.Groups[idx+1:]...)
		if toIndex < 0 {
			toIndex = 0
		}
		if toIndex > len(rest) {
			toIndex = len(rest)
		}
		rest = append(rest, config.Group{})
		copy(rest[toIndex+1:], rest[toIndex:])
		rest[toIndex] = group
		cfg.Groups = rest
		return nil
	})
}

// UndoDeleteItem restores the item removed by the last DeleteItem call.
func (a *App) UndoDeleteItem() (string, error) {
	a.undoMu.Lock()
	snapshot := a.deletedItem
	a.undoMu.Unlock()

	if snapshot == nil {
		return "", errors.New("没有可撤销的删除")
	}

	mgr := a.configManager()
	if mgr == nil {
		return "", errors.New("配置管理器尚未就绪")
	}

	err := mgr.Update(func(cfg *config.Config) error {
		idx := findGroupIndex(cfg.Groups, snapshot.groupID)
		if idx < 0 {
			return fmt.Errorf("分组「%s」已被删除，无法恢复该项目", snapshot.groupID)
		}
		for _, item := range cfg.Groups[idx].Items {
			if item.ID == snapshot.item.ID {
				return nil
			}
		}
		items := cfg.Groups[idx].Items
		pos := snapshot.index
		if pos < 0 || pos > len(items) {
			pos = len(items)
		}
		items = append(items, config.Item{})
		copy(items[pos+1:], items[pos:])
		items[pos] = snapshot.item
		cfg.Groups[idx].Items = items
		return nil
	})
	if err != nil {
		return "", err
	}
	a.consumeDeletedItem(snapshot)
	a.emitToast("success", fmt.Sprintf("已恢复「%s」", snapshot.item.Name))
	return snapshot.item.Name, nil
}

// consumeDeletedItem forgets an undo snapshot once the restore reached the disk. A refused write
// (a broken config.json, a blocked file) must leave Ctrl+Z able to try again instead of dropping
// the only copy of what was deleted.
func (a *App) consumeDeletedItem(snapshot *deletedItemSnapshot) {
	a.undoMu.Lock()
	if a.deletedItem == snapshot {
		a.deletedItem = nil
	}
	a.undoMu.Unlock()
}

// consumeDeletedGroup is consumeDeletedItem for group snapshots.
func (a *App) consumeDeletedGroup(snapshot *deletedGroupSnapshot) {
	a.undoMu.Lock()
	if a.deletedGroup == snapshot {
		a.deletedGroup = nil
	}
	a.undoMu.Unlock()
}

// UndoDeleteGroup restores the group removed by the last DeleteGroup call.
func (a *App) UndoDeleteGroup() (string, error) {
	a.undoMu.Lock()
	snapshot := a.deletedGroup
	a.undoMu.Unlock()

	if snapshot == nil {
		return "", errors.New("没有可撤销的删除")
	}

	mgr := a.configManager()
	if mgr == nil {
		return "", errors.New("配置管理器尚未就绪")
	}

	err := mgr.Update(func(cfg *config.Config) error {
		if findGroupIndex(cfg.Groups, snapshot.group.ID) >= 0 {
			return nil
		}
		pos := snapshot.index
		if pos < 0 || pos > len(cfg.Groups) {
			pos = len(cfg.Groups)
		}
		groups := cfg.Groups
		groups = append(groups, config.Group{})
		copy(groups[pos+1:], groups[pos:])
		groups[pos] = snapshot.group
		cfg.Groups = groups
		return nil
	})
	if err != nil {
		return "", err
	}
	a.consumeDeletedGroup(snapshot)
	a.SetActiveGroup(snapshot.group.ID)
	a.emitToast("success", fmt.Sprintf("已恢复分组「%s」", snapshot.group.Name))
	return snapshot.group.Name, nil
}

// RelocateItem asks for a new target for an item whose file moved, then updates the path and the
// icon that was derived from it. It returns the new path, or "" when the user cancelled.
func (a *App) RelocateItem(groupID, itemID string) (string, error) {
	ctx := a.context()
	if ctx == nil {
		return "", errors.New("窗口尚未就绪")
	}
	mgr := a.configManager()
	if mgr == nil {
		return "", errors.New("配置管理器尚未就绪")
	}

	item, ok := findItem(mgr.Get(), groupID, itemID)
	if !ok {
		return "", errors.New("项目不存在")
	}

	picked, err := runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
		Title:            "选择新的目标",
		DefaultDirectory: filepath.Dir(item.Path),
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(picked) == "" {
		return "", nil
	}

	if err := mgr.Update(func(cfg *config.Config) error {
		gIdx := findGroupIndex(cfg.Groups, groupID)
		if gIdx < 0 {
			return errors.New("分组不存在")
		}
		items := cfg.Groups[gIdx].Items
		for i := range items {
			if items[i].ID != itemID {
				continue
			}
			// Only a derived icon follows the new target; a custom icon is left alone.
			if items[i].Icon == "" || strings.EqualFold(items[i].Icon, items[i].Path) {
				items[i].Icon = picked
			}
			items[i].Path = picked
			return nil
		}
		return errors.New("项目不存在")
	}); err != nil {
		return "", err
	}

	a.emitToast("success", "已更新目标路径")
	return picked, nil
}

// pendingUndoLabel describes the undo snapshot that Ctrl+Z would apply, or "" when there is none.
func (a *App) pendingUndoLabel() string {
	a.undoMu.Lock()
	defer a.undoMu.Unlock()
	switch {
	case a.deletedItem != nil:
		return "item:" + a.deletedItem.item.Name
	case a.deletedGroup != nil:
		return "group:" + a.deletedGroup.group.Name
	default:
		return ""
	}
}

// ScanStartMenu lists shortcuts from the current user's and the machine's Start Menu.
func (a *App) ScanStartMenu() []resolver.StartMenuEntry {
	return resolver.ScanStartMenu()
}

// ImportStartMenuItems adds the given shortcut files (.lnk) to a group, skipping duplicates.
// It returns how many items were added.
func (a *App) ImportStartMenuItems(groupID string, lnkPaths []string) (int, error) {
	mgr := a.configManager()
	if mgr == nil {
		return 0, errors.New("配置管理器尚未就绪")
	}
	if len(lnkPaths) == 0 {
		return 0, nil
	}
	if len(lnkPaths) > maxImportCount {
		lnkPaths = lnkPaths[:maxImportCount]
	}

	cfg := a.GetConfig()
	existing := make(map[string]bool, len(cfg.Groups)*4)
	for _, group := range cfg.Groups {
		for _, item := range group.Items {
			existing[strings.ToLower(item.Path+"|"+item.Args)] = true
		}
	}

	added := make([]config.Item, 0, len(lnkPaths))
	for _, lnkPath := range lnkPaths {
		resolved, err := resolver.ResolvePath(strings.TrimSpace(lnkPath))
		if err != nil || resolved.Path == "" {
			continue
		}
		key := strings.ToLower(resolved.Path + "|" + resolved.Args)
		if existing[key] {
			continue
		}
		existing[key] = true

		iconSrc := resolved.IconSrc
		if iconSrc == "" {
			iconSrc = resolved.Path
		}
		iconRel, iconErr := resolver.ExtractAndCacheIcon(iconSrc, a.iconsDir)
		if iconErr != nil && iconSrc != resolved.Path {
			iconRel, iconErr = resolver.ExtractAndCacheIcon(resolved.Path, a.iconsDir)
		}
		if iconErr != nil {
			iconRel = ""
		}

		added = append(added, config.Item{
			ID:      generateID("item"),
			Name:    resolved.Name,
			Path:    resolved.Path,
			Args:    resolved.Args,
			WorkDir: resolved.WorkDir,
			Icon:    iconRel,
		})
	}

	if len(added) == 0 {
		return 0, nil
	}

	err := mgr.Update(func(cfg *config.Config) error {
		idx := findGroupIndex(cfg.Groups, groupID)
		if idx < 0 && len(cfg.Groups) > 0 {
			// Dropping imports into a group the user did not choose hides them; fail instead.
			return fmt.Errorf("未找到分组 %q", groupID)
		}
		if idx < 0 {
			cfg.Groups = append(cfg.Groups, config.Group{ID: "default", Name: "常用工具", Items: []config.Item{}})
			idx = 0
		}
		cfg.Groups[idx].Items = append(cfg.Groups[idx].Items, added...)
		return nil
	})
	if err != nil {
		return 0, err
	}

	a.emitToast("success", fmt.Sprintf("已导入 %d 个开始菜单项", len(added)))
	return len(added), nil
}

// itemSnapshot records where an item sits so a delete can be undone.
func (a *App) itemSnapshot(groupID, itemID string) *deletedItemSnapshot {
	cfg := a.GetConfig()
	idx := findGroupIndex(cfg.Groups, groupID)
	if idx < 0 {
		return nil
	}
	for i, item := range cfg.Groups[idx].Items {
		if item.ID == itemID {
			return &deletedItemSnapshot{groupID: groupID, index: i, item: item}
		}
	}
	return nil
}

// groupSnapshot records a group's position so a delete can be undone.
func (a *App) groupSnapshot(groupID string) *deletedGroupSnapshot {
	cfg := a.GetConfig()
	idx := findGroupIndex(cfg.Groups, groupID)
	if idx < 0 {
		return nil
	}
	return &deletedGroupSnapshot{index: idx, group: cfg.Groups[idx]}
}

// applyAutoStart writes the registry entry to match the saved preference.
func applyAutoStart(enabled bool) error {
	return autostart.Set(enabled)
}

// GetAppInfo reports paths, version, and whether autostart is registered.
func (a *App) GetAppInfo() AppInfo {
	info := AppInfo{
		Version:      appVersion,
		BaseDir:      a.baseDir,
		ConfigPath:   a.configPath,
		ConfigBackup: config.BackupPath(a.configPath),
		IconsDir:     a.iconsDir,
		LogPath:      applog.LogPath(a.baseDir),
	}
	if exe, err := autostart.CurrentExe(); err == nil {
		info.ExePath = exe
	}
	if enabled, err := autostart.Enabled(); err != nil {
		info.AutoStartError = err.Error()
	} else {
		info.AutoStart = enabled
	}
	return info
}

// SetAutoStart registers or removes the per-user Run entry and stores the preference.
func (a *App) SetAutoStart(enabled bool) error {
	if err := autostart.Set(enabled); err != nil {
		return err
	}
	if mgr := a.configManager(); mgr != nil {
		if err := mgr.Update(func(cfg *config.Config) error {
			cfg.Settings.AutoStart = enabled
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

// syncAutostart keeps the registry entry in line with the saved preference, which also repairs
// the registered path after the executable was moved.
func (a *App) syncAutostart(cfg config.Config) {
	if enabled, err := autostart.Enabled(); err == nil && enabled == cfg.Settings.AutoStart {
		return
	}
	if err := autostart.Set(cfg.Settings.AutoStart); err != nil {
		log.Printf("[autostart] sync failed: %v", err)
	}
}

// GetConfigError returns the current config.json problem, or an empty string when it is healthy.
// The UI shows it as a banner, so a broken file is visible without a modal dialog.
func (a *App) GetConfigError() string {
	a.errMu.Lock()
	defer a.errMu.Unlock()
	return a.lastConfigErr
}

// GetUndoLabel is kept for the frontend: it returns the pending undo description or "".
func (a *App) GetUndoLabel() string {
	label := a.pendingUndoLabel()
	if label == "" {
		return ""
	}
	if name, found := strings.CutPrefix(label, "item:"); found {
		return "撤销删除「" + name + "」"
	}
	if name, found := strings.CutPrefix(label, "group:"); found {
		return "撤销删除分组「" + name + "」"
	}
	return ""
}

// PendingUndoKind reports what Ctrl+Z would restore: "item", "group", or "" when nothing is
// pending. The label above is prose meant for humans, so the UI cannot read the kind back out of
// it and needs this separate answer to pick the right restore call.
func (a *App) PendingUndoKind() string {
	switch label := a.pendingUndoLabel(); {
	case strings.HasPrefix(label, "item:"):
		return "item"
	case strings.HasPrefix(label, "group:"):
		return "group"
	default:
		return ""
	}
}

// OpenBaseDir opens the folder that holds config.json in Explorer.
func (a *App) OpenBaseDir() error {
	return launcher.OpenFile(a.baseDir)
}

// ExportConfig writes the current configuration to a file the user picks.
func (a *App) ExportConfig() (string, error) {
	target, err := a.saveFileDialog("导出 JeeDesk 配置", "config.json")
	if err != nil || target == "" {
		return "", err
	}
	if err := config.Save(target, a.GetConfig()); err != nil {
		return "", err
	}
	a.emitToast("success", "配置已导出")
	return target, nil
}

// ImportConfig replaces the current configuration with the one in a file the user picks.
func (a *App) ImportConfig() (int, error) {
	source, err := a.openFileDialog("导入 JeeDesk 配置")
	if err != nil || source == "" {
		return 0, err
	}

	imported, err := config.Load(source)
	if err != nil {
		return 0, fmt.Errorf("配置文件无法解析: %w", err)
	}
	if len(imported.Groups) == 0 {
		return 0, errors.New("该配置文件不含任何分组")
	}

	mgr := a.configManager()
	if mgr == nil {
		return 0, errors.New("配置管理器尚未就绪")
	}
	// Replace, not Update: importing a file the user picked is the explicit way out of the write
	// block that a damaged config.json puts the application in.
	if err := mgr.Replace(imported); err != nil {
		return 0, err
	}

	importedCfg := a.GetConfig()
	a.syncHooks(importedCfg.Settings, true)
	// The imported settings carry auto_start; without this the registry keeps the old value while
	// the settings page shows the new one.
	a.syncAutostart(importedCfg)
	a.emitToast("success", fmt.Sprintf("已导入 %d 个分组", len(imported.Groups)))
	return len(imported.Groups), nil
}

// ApplyWindowState moves the window to the saved position, keeping it on a visible monitor.
// The frontend calls it once after mounting.
func (a *App) ApplyWindowState() {
	cfg := a.GetConfig()
	go func() {
		defer applog.Recover("app.applyWindowState")
		time.Sleep(60 * time.Millisecond)
		if a.findMainHwndFn == nil {
			return
		}
		hwnd := a.findMainHwndFn()
		if hwnd == 0 {
			return
		}
		if !cfg.Settings.WindowPositioned {
			// No saved position: appear on the monitor the cursor is on, which is where the user
			// pressed the hotkey when several screens are attached.
			centerOnCursorMonitor(hwnd)
			return
		}
		moveWindowTo(hwnd, int32(cfg.Settings.WindowX), int32(cfg.Settings.WindowY))
		ensureWindowReachable(hwnd)
	}()
}

// SaveWindowState remembers the window position after a drag or resize. Writing is debounced
// because the window emits many move events while the user drags it.
func (a *App) SaveWindowState() {
	a.windowSaveMu.Lock()
	if a.windowSaveTimer != nil {
		a.windowSaveTimer.Stop()
	}
	a.windowSaveTimer = time.AfterFunc(windowSaveDelay, a.flushWindowState)
	a.windowSaveMu.Unlock()
}

// flushWindowState writes the pending window geometry without notifying the UI.
func (a *App) flushWindowState() {
	defer applog.Recover("app.flushWindowState")
	a.windowSaveMu.Lock()
	if a.windowSaveTimer != nil {
		a.windowSaveTimer.Stop()
		a.windowSaveTimer = nil
	}
	a.windowSaveMu.Unlock()

	mgr := a.configManager()
	if mgr == nil || a.findMainHwndFn == nil {
		return
	}
	hwnd := a.findMainHwndFn()
	if hwnd == 0 {
		return
	}
	rect, ok := windowRectOf(hwnd)
	if !ok {
		return
	}

	// GetWindowRect answers in physical pixels, while window_width/window_height are device
	// independent pixels (they are handed to Wails unchanged at startup). Storing the raw rectangle
	// made the panel grow by the scale factor on every restart of a scaled display.
	dpi := windowDPI(hwnd)
	width := dipOf(int(rect.Right-rect.Left), dpi)
	height := dipOf(int(rect.Bottom-rect.Top), dpi)
	x := int(rect.Left)
	y := int(rect.Top)

	cfg := a.GetConfig()
	if cfg.Settings.WindowPositioned && cfg.Settings.WindowX == x && cfg.Settings.WindowY == y &&
		cfg.Settings.WindowWidth == width && cfg.Settings.WindowHeight == height {
		return
	}

	if err := mgr.UpdateQuiet(func(next *config.Config) error {
		next.Settings.WindowX = x
		next.Settings.WindowY = y
		next.Settings.WindowWidth = width
		next.Settings.WindowHeight = height
		next.Settings.WindowPositioned = true
		return nil
	}); err != nil {
		log.Printf("[window] saving geometry failed: %v", err)
	}
}

// ResetWindowState clears the saved geometry so the window returns to its default size.
func (a *App) ResetWindowState() error {
	mgr := a.configManager()
	if mgr == nil {
		return errors.New("配置管理器尚未就绪")
	}
	return mgr.Update(func(cfg *config.Config) error {
		cfg.Settings.WindowX = 0
		cfg.Settings.WindowY = 0
		cfg.Settings.WindowWidth = 0
		cfg.Settings.WindowHeight = 0
		cfg.Settings.WindowPositioned = false
		return nil
	})
}

// lookupItem finds a group and item by ID.
func lookupItem(cfg config.Config, groupID, itemID string) (string, config.Group, config.Item, bool) {
	idx := findGroupIndex(cfg.Groups, groupID)
	if idx < 0 {
		return "", config.Group{}, config.Item{}, false
	}
	group := cfg.Groups[idx]
	for _, item := range group.Items {
		if item.ID == itemID {
			return group.ID, group, item, true
		}
	}
	return "", config.Group{}, config.Item{}, false
}

// withDialog suppresses hide_on_blur while a native dialog is open.
func (a *App) withDialog(open func() (string, error)) (string, error) {
	a.nativeDialogOpen.Store(true)
	defer func() {
		a.stateMu.Lock()
		a.lastShown = time.Now()
		a.stateMu.Unlock()
		a.nativeDialogOpen.Store(false)
	}()
	return open()
}

// saveFileDialog asks for a destination path for the configuration export.
func (a *App) saveFileDialog(title, defaultName string) (string, error) {
	ctx := a.context()
	if ctx == nil {
		return "", errors.New("应用上下文未初始化")
	}
	return a.withDialog(func() (string, error) {
		return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
			Title:           title,
			DefaultFilename: defaultName,
			Filters: []runtime.FileFilter{
				{DisplayName: "JSON (*.json)", Pattern: "*.json"},
			},
		})
	})
}

// openFileDialog asks for a configuration file to import.
func (a *App) openFileDialog(title string) (string, error) {
	ctx := a.context()
	if ctx == nil {
		return "", errors.New("应用上下文未初始化")
	}
	return a.withDialog(func() (string, error) {
		return runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{
			Title: title,
			Filters: []runtime.FileFilter{
				{DisplayName: "JSON (*.json)", Pattern: "*.json"},
			},
		})
	})
}
