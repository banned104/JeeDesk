package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"jeedesk/internal/applog"
	"jeedesk/internal/config"
	"jeedesk/internal/launcher"
	"jeedesk/internal/resolver"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	// updateEndpoint is the GitHub releases API of the project that publishes JeeDesk.
	updateEndpoint = "https://api.github.com/repos/banned104/JeeDesk/releases/latest"
	// updateTimeout keeps the settings dialog from hanging on a dead network.
	updateTimeout = 8 * time.Second
	// orphanIconAge is how long a freshly written icon is exempt from cleanup. An icon written for
	// an item that is about to be saved must not be deleted in the same second.
	orphanIconAge = 10 * time.Minute
)

// UpdateInfo is the result of a manual update check.
type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	URL       string `json:"url"`
	Notes     string `json:"notes"`
	Available bool   `json:"available"`
	CheckedAt int64  `json:"checked_at"`
	Error     string `json:"error"`
}

// SetModalOpen tells the backend that a frontend dialog is on screen. While it is, losing focus
// must not hide the window: a native file picker or a click on another window would otherwise
// close the dialog and drop the form.
func (a *App) SetModalOpen(open bool) {
	a.uiModalOpen.Store(open)
}

// OpenURL hands an http(s) link to the default browser. window.open inside the WebView would open
// another frameless WebView window instead of the system browser.
func (a *App) OpenURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return errors.New("只允许打开 http/https 链接")
	}
	ctx := a.context()
	if ctx == nil {
		return errors.New("应用上下文未初始化")
	}
	runtime.BrowserOpenURL(ctx, trimmed)
	return nil
}

// GetMissingTargets returns the IDs of items whose target no longer exists, so the grid can mark
// them before the user clicks a broken shortcut.
func (a *App) GetMissingTargets() []string {
	cfg := a.GetConfig()
	missing := make([]string, 0)
	for _, group := range cfg.Groups {
		for _, item := range group.Items {
			if targetExists(item.Path) {
				continue
			}
			missing = append(missing, item.ID)
		}
	}
	sort.Strings(missing)
	return missing
}

// targetExists reports whether path points at something launchable. URIs are always considered
// valid; a plain path is checked on disk.
func targetExists(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "://") {
		return true
	}
	if filepath.VolumeName(trimmed) == "" {
		return true
	}
	_, err := os.Stat(trimmed)
	return err == nil
}

// cachedIconPresent reports whether icon still resolves to a file in the icon cache. An icon that
// lives outside the cache (a PNG the user picked) is not ours to rebuild, so it counts as present.
func cachedIconPresent(iconsDir string, icon string) bool {
	icon = strings.TrimSpace(icon)
	if icon == "" {
		return false
	}
	rel, inCache := strings.CutPrefix(strings.ReplaceAll(icon, "\\", "/"), "icons/")
	if !inCache {
		return true
	}
	_, err := os.Stat(filepath.Join(iconsDir, filepath.Base(rel)))
	return err == nil
}

// EnsureIcons extracts an icon for every item that has none, so a hand-written config.json or an
// item added before the cache existed still shows its real icon. It runs in the background: icon
// extraction touches the disk and must not block the first paint.
func (a *App) EnsureIcons() (int, error) {
	mgr := a.configManager()
	if mgr == nil {
		return 0, errors.New("配置管理器尚未就绪")
	}

	type filledIcon struct {
		groupID string
		itemID  string
		icon    string
	}

	filled := make([]filledIcon, 0)
	for _, group := range mgr.Get().Groups {
		for _, item := range group.Items {
			// An item whose cached icon file is gone needs a new one: bumping iconCacheVersion wipes
			// the cache, and skipping every item with a non-empty Icon left the whole grid on the
			// bundled fallback until each entry was edited by hand.
			if cachedIconPresent(a.iconsDir, item.Icon) || !targetExists(item.Path) {
				continue
			}
			rel, err := resolver.ExtractAndCacheIcon(item.Path, a.iconsDir)
			if err != nil {
				log.Printf("[icons] extract for %q failed: %v", item.Path, err)
				continue
			}
			filled = append(filled, filledIcon{groupID: group.ID, itemID: item.ID, icon: rel})
		}
	}

	if len(filled) == 0 {
		return 0, nil
	}

	// Written quietly: the icons are a side effect of the paths already on screen, and a full
	// config:updated would rebuild the whole grid for a cosmetic change.
	if err := mgr.UpdateQuiet(func(cfg *config.Config) error {
		byGroup := make(map[string]map[string]string, len(filled))
		for _, entry := range filled {
			items := byGroup[entry.groupID]
			if items == nil {
				items = make(map[string]string)
				byGroup[entry.groupID] = items
			}
			items[entry.itemID] = entry.icon
		}
		for gi := range cfg.Groups {
			items := byGroup[cfg.Groups[gi].ID]
			if items == nil {
				continue
			}
			for ii := range cfg.Groups[gi].Items {
				if next, found := items[cfg.Groups[gi].Items[ii].ID]; found && cfg.Groups[gi].Items[ii].Icon == "" {
					cfg.Groups[gi].Items[ii].Icon = next
				}
			}
		}
		return nil
	}); err != nil {
		return 0, err
	}

	log.Printf("[icons] filled %d missing item icons", len(filled))
	a.emitEvent("config:updated", mgr.Get())
	return len(filled), nil
}

// iconCacheVersion is bumped whenever icon extraction changes in a way that makes previously
// written PNGs wrong (a new strategy, a fixed conversion). Cached files from another version are
// dropped once at startup so nobody keeps staring at a blank icon.
const iconCacheVersion = "2"

// syncIconCacheVersion drops cached icons written by an older extractor.
func (a *App) syncIconCacheVersion() error {
	marker := filepath.Join(a.iconsDir, ".version")
	if data, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(data)) == iconCacheVersion {
		return nil
	}
	entries, err := os.ReadDir(a.iconsDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".png") {
			continue
		}
		if err := os.Remove(filepath.Join(a.iconsDir, entry.Name())); err == nil {
			removed++
		}
	}
	if err := os.MkdirAll(a.iconsDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(marker, []byte(iconCacheVersion), 0o644); err != nil {
		return err
	}
	if removed > 0 {
		log.Printf("[icons] dropped %d cached icons from an older extractor", removed)
	}
	return nil
}

// CleanupIcons deletes cached icons that no item references any more and returns how many files
// were removed. Icons younger than orphanIconAge are kept, because they may belong to an item that
// is still being added.
func (a *App) CleanupIcons() (int, error) {
	// With a broken config.json the in-memory configuration is the default one, which references
	// no icons at all: cleaning then would delete every cached file. Refuse until the file loads.
	if a.configLoadErr != nil {
		return 0, nil
	}
	entries, err := os.ReadDir(a.iconsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}

	cfg := a.GetConfig()
	if len(cfg.Groups) == 0 {
		return 0, nil
	}

	referenced := make(map[string]bool)
	for _, group := range cfg.Groups {
		for _, item := range group.Items {
			if name := iconBaseName(item.Icon); name != "" {
				referenced[name] = true
			}
		}
	}

	removed := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".png") {
			continue
		}
		if referenced[entry.Name()] {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < orphanIconAge {
			continue
		}
		if err := os.Remove(filepath.Join(a.iconsDir, entry.Name())); err == nil {
			removed++
		}
	}
	if removed > 0 {
		log.Printf("[icons] removed %d orphaned icon files", removed)
	}
	return removed, nil
}

// iconBaseName returns the file name of a cached icon reference ("icons/x.png" -> "x.png").
func iconBaseName(icon string) string {
	trimmed := strings.TrimSpace(icon)
	if !strings.HasPrefix(trimmed, "icons/") {
		return ""
	}
	return filepath.Base(trimmed)
}

// SetCollapsedGroups stores which sidebar groups are folded away. It writes quietly: the UI
// already knows, and a full config:updated round trip would re-render the list that was just
// clicked.
func (a *App) SetCollapsedGroups(ids []string) error {
	mgr := a.configManager()
	if mgr == nil {
		return errors.New("配置管理器尚未就绪")
	}
	cleaned := make([]string, 0, len(ids))
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		cleaned = append(cleaned, trimmed)
	}
	return mgr.UpdateQuiet(func(cfg *config.Config) error {
		cfg.Settings.CollapsedGroups = cleaned
		return nil
	})
}

// RebuildConfigFile writes the in-memory configuration back to config.json. It is the recovery
// action offered when the file was deleted or cannot be parsed, so it must work precisely when
// regular writes are blocked.
func (a *App) RebuildConfigFile() error {
	mgr := a.configManager()
	if mgr == nil {
		return errors.New("配置管理器尚未就绪")
	}
	return mgr.Replace(a.GetConfig())
}

// OpenLogFile opens logs/app.log in the default editor.
func (a *App) OpenLogFile() error {
	return launcher.OpenFile(applog.LogPath(a.baseDir))
}

// OpenCrashReports opens the folder that holds crash reports and the runtime's crash logs. It is
// the answer to "上次没有正常退出" when the user wants to look at what happened.
func (a *App) OpenCrashReports() error {
	dir := applog.CrashDir(a.baseDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return launcher.OpenFile(dir)
}

// ReportFrontendError records an error the WebView could not handle: a Vue render failure, an
// uncaught exception, or a rejected promise. The UI cannot write files, so it hands the report to
// the backend, which appends it to logs/crashes with the rest of the crash evidence.
func (a *App) ReportFrontendError(kind, message, stack, detail string) {
	applog.WriteFrontendReport(kind, message, stack, detail)
}

// CheckUpdates asks the GitHub releases API for the newest published version. It only reports;
// nothing is downloaded or installed.
func (a *App) CheckUpdates() UpdateInfo {
	info := UpdateInfo{Current: appVersion, CheckedAt: time.Now().Unix()}

	client := &http.Client{Timeout: updateTimeout}
	req, err := http.NewRequest(http.MethodGet, updateEndpoint, nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "JeeDesk/"+appVersion)

	resp, err := client.Do(req)
	if err != nil {
		info.Error = fmt.Sprintf("无法连接更新服务: %v", err)
		return info
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		info.Error = "尚未发布任何版本"
		return info
	}
	if resp.StatusCode != http.StatusOK {
		info.Error = fmt.Sprintf("更新服务返回 %d", resp.StatusCode)
		return info
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&payload); err != nil {
		info.Error = fmt.Sprintf("更新信息无法解析: %v", err)
		return info
	}

	info.Latest = strings.TrimSpace(strings.TrimPrefix(payload.TagName, "v"))
	info.URL = payload.HTMLURL
	info.Notes = strings.TrimSpace(payload.Body)
	if info.Latest == "" {
		info.Error = "更新信息缺少版本号"
		return info
	}
	info.Available = compareVersions(info.Latest, appVersion) > 0
	return info
}

// compareVersions compares dotted numeric versions and returns 1 when a is newer, -1 when it is
// older, and 0 when they are equal. A non-numeric segment compares as 0 so a pre-release suffix
// does not look newer than the release.
func compareVersions(a, b string) int {
	split := func(v string) []string {
		cleaned := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(v), "v"))
		if idx := strings.IndexAny(cleaned, "-+"); idx >= 0 {
			cleaned = cleaned[:idx]
		}
		return strings.Split(cleaned, ".")
	}

	left, right := split(a), split(b)
	size := len(left)
	if len(right) > size {
		size = len(right)
	}
	for i := 0; i < size; i++ {
		var lv, rv int
		if i < len(left) {
			lv = atoiSafe(left[i])
		}
		if i < len(right) {
			rv = atoiSafe(right[i])
		}
		if lv != rv {
			if lv > rv {
				return 1
			}
			return -1
		}
	}
	return 0
}

// atoiSafe parses a version segment, treating anything unexpected as zero.
func atoiSafe(segment string) int {
	value := 0
	for i := 0; i < len(segment); i++ {
		c := segment[i]
		if c < '0' || c > '9' {
			break
		}
		value = value*10 + int(c-'0')
	}
	return value
}
