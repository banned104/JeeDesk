package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"jeedesk/internal/config"
	"jeedesk/internal/hook"
	"jeedesk/internal/tray"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	app := NewApp(dir)

	mgr, err := config.NewManagerWithFallback(app.configPath, app.onConfigUpdated, app.onConfigError)
	if err != nil {
		t.Fatalf("NewManagerWithFallback failed: %v", err)
	}
	app.setConfigManager(mgr)
	t.Cleanup(func() {
		_ = mgr.Close()
	})

	cfg := app.GetConfig()
	if len(cfg.Groups) > 0 {
		app.SetActiveGroup(cfg.Groups[0].ID)
	}
	return app
}

func TestAppGroupAndItemLifecycle(t *testing.T) {
	app := newTestApp(t)

	devGroupID, err := app.AddGroup("开发工具")
	if err != nil {
		t.Fatalf("AddGroup failed: %v", err)
	}

	cfg := app.GetConfig()
	if len(cfg.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(cfg.Groups))
	}
	devGroup := cfg.Groups[1]
	// The returned ID is the new group, which also becomes the target for file drops.
	if devGroup.ID != devGroupID || app.getActiveGroupID() != devGroupID {
		t.Fatalf("expected the new group to be returned and active, got id %q, returned %q, active %q",
			devGroup.ID, devGroupID, app.getActiveGroupID())
	}
	if devGroup.Name != "开发工具" {
		t.Fatalf("expected group name 开发工具, got %q", devGroup.Name)
	}

	if err := app.RenameGroup(devGroup.ID, "Dev Tools"); err != nil {
		t.Fatalf("RenameGroup failed: %v", err)
	}
	if app.GetConfig().Groups[1].Name != "Dev Tools" {
		t.Fatalf("rename did not persist")
	}

	notepad := `C:\Windows\System32\notepad.exe`
	if err := app.AddDroppedFiles(devGroup.ID, []string{notepad}); err != nil {
		t.Fatalf("AddDroppedFiles failed: %v", err)
	}

	cfg = app.GetConfig()
	if len(cfg.Groups[1].Items) != 1 {
		t.Fatalf("expected 1 item after drop, got %d", len(cfg.Groups[1].Items))
	}
	added := cfg.Groups[1].Items[0]
	if !strings.EqualFold(added.Path, notepad) {
		t.Fatalf("unexpected dropped path %q", added.Path)
	}
	if !strings.HasPrefix(added.Icon, "icons/") || !strings.HasSuffix(added.Icon, ".png") {
		t.Fatalf("expected cached icon path icons/<hash>.png, got %q", added.Icon)
	}
	cachedFile := filepath.Join(app.iconsDir, filepath.Base(added.Icon))
	if _, err := os.Stat(cachedFile); err != nil {
		t.Fatalf("cached icon file missing at %s: %v", cachedFile, err)
	}

	added.Name = "My Notepad"
	added.Args = "--test-arg"
	added.WorkDir = `C:\Windows`
	if err := app.UpdateItem(devGroup.ID, added); err != nil {
		t.Fatalf("UpdateItem failed: %v", err)
	}
	updated := app.GetConfig().Groups[1].Items[0]
	if updated.Name != "My Notepad" || updated.Args != "--test-arg" || updated.WorkDir != `C:\Windows` {
		t.Fatalf("UpdateItem did not persist fields: %+v", updated)
	}

	if err := app.DeleteItem(devGroup.ID, added.ID); err != nil {
		t.Fatalf("DeleteItem failed: %v", err)
	}
	if len(app.GetConfig().Groups[1].Items) != 0 {
		t.Fatalf("expected 0 items after DeleteItem")
	}

	if err := app.DeleteGroup(devGroup.ID); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}
	if len(app.GetConfig().Groups) != 1 {
		t.Fatalf("expected 1 group after DeleteGroup")
	}
}

func TestAppSaveSettingsValidation(t *testing.T) {
	app := newTestApp(t)

	if err := app.SaveSettings(config.Settings{
		Hotkey:       "NotAKey+???",
		MouseTrigger: "none",
	}); err == nil {
		t.Fatal("expected error for invalid hotkey")
	}

	if err := app.SaveSettings(config.Settings{
		Hotkey:       "Alt+G",
		MouseTrigger: "invalid_btn",
	}); err == nil {
		t.Fatal("expected error for invalid mouse trigger")
	}

	if err := app.SaveSettings(config.Settings{
		Hotkey:       "Ctrl+Shift+G",
		MouseTrigger: "xbutton2",
		HideOnLaunch: false,
		HideOnBlur:   true,
	}); err != nil {
		t.Fatalf("expected valid settings to save, got: %v", err)
	}

	got := app.GetConfig().Settings
	if got.Hotkey != "Ctrl+Shift+G" || got.MouseTrigger != "xbutton2" || got.HideOnLaunch {
		t.Fatalf("saved settings mismatch: %+v", got)
	}
}

func TestAppBrokenConfigResilienceAndHotReload(t *testing.T) {
	app := newTestApp(t)
	if err := app.configManager().StartWatching(); err != nil {
		t.Fatalf("StartWatching failed: %v", err)
	}

	if _, err := app.AddGroup("KeepMe"); err != nil {
		t.Fatalf("AddGroup failed: %v", err)
	}

	// Corrupt config.json externally.
	if err := os.WriteFile(app.configPath, []byte(`{"settings": { broken json`), 0o644); err != nil {
		t.Fatalf("write broken json: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for app.GetConfigError() == "" && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if app.GetConfigError() == "" {
		t.Fatal("expected the config error to be reported on malformed config.json")
	}

	// Last valid config must still be intact in memory.
	if len(app.GetConfig().Groups) != 2 {
		t.Fatalf("expected last valid config (2 groups) to remain intact, got %d", len(app.GetConfig().Groups))
	}

	// Fix config.json externally and verify hot reload recovers.
	fixed := `{
  "settings": {
    "hotkey": "Alt+K",
    "mouse_trigger": "middle",
    "hide_on_launch": true,
    "hide_on_blur": false
  },
  "groups": [
    { "id": "recovered", "name": "已恢复", "items": [] }
  ]
}`
	if err := os.WriteFile(app.configPath, []byte(fixed), 0o644); err != nil {
		t.Fatalf("write fixed json: %v", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cfg := app.GetConfig()
		if len(cfg.Groups) == 1 && cfg.Groups[0].ID == "recovered" && cfg.Settings.Hotkey == "Alt+K" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("config did not hot-reload after fixing JSON; got %+v", app.GetConfig())
}

func TestIconMiddlewareSecurityAndServing(t *testing.T) {
	dir := t.TempDir()
	iconsDir := filepath.Join(dir, "icons")
	if err := os.MkdirAll(iconsDir, 0o755); err != nil {
		t.Fatalf("mkdir icons: %v", err)
	}

	samplePNG := filepath.Join(iconsDir, "sample.png")
	if err := os.WriteFile(samplePNG, []byte("\x89PNG\r\n\x1a\n"), 0o644); err != nil {
		t.Fatalf("write sample png: %v", err)
	}
	secretFile := filepath.Join(dir, "config.json")
	if err := os.WriteFile(secretFile, []byte(`{"secret":true}`), 0o644); err != nil {
		t.Fatalf("write secret file: %v", err)
	}
	customICO := filepath.Join(dir, "custom.ico")
	if err := os.WriteFile(customICO, tray.IconBytes(), 0o644); err != nil {
		t.Fatalf("write custom ico: %v", err)
	}

	fallbackCalled := false
	mw := newIconMiddleware(iconsDir)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fallbackCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	// 1. Valid cached icon.
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/icons/sample.png?v=123", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for /icons/sample.png, got %d", rec.Code)
	}

	// 2. Path traversal blocked.
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/icons/..%5Cconfig.json", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for traversal attempt, got %d", rec.Code)
	}

	// 3. Valid custom icon.
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/custom-icon?path="+url.QueryEscape(customICO), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for custom ico, got %d", rec.Code)
	}

	// 4. Non-image custom icon forbidden.
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/custom-icon?path="+url.QueryEscape(secretFile), nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-image custom icon, got %d", rec.Code)
	}

	// 5. Normal asset passes through to next handler.
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/index.html", nil))
	if !fallbackCalled {
		t.Fatal("expected non-icon path to delegate to next handler")
	}
}

func newBlurTestApp(t *testing.T) *App {
	t.Helper()
	app := newTestApp(t)
	app.stateMu.Lock()
	app.visible = true
	app.lastShown = time.Now().Add(-2 * blurGracePeriod)
	app.stateMu.Unlock()

	app.findMainHwndFn = func() uintptr { return 0x1234 }
	app.isVisibleFn = func(uintptr) bool { return true }
	app.isMouseDownFn = func() bool { return false }
	app.isProcessFgFn = func() bool { return false }
	app.isCursorInWinFn = func(uintptr) bool { return false }
	app.focusWindowFn = func(uintptr) {}
	return app
}

func TestShouldHideOnBlur(t *testing.T) {
	t.Run("hides_when_unfocused_outside_and_mouse_up", func(t *testing.T) {
		app := newBlurTestApp(t)
		if !app.shouldHideOnBlur() {
			t.Fatal("expected shouldHideOnBlur() to return true")
		}
		app.OnWindowBlur()
		app.stateMu.Lock()
		vis := app.visible
		app.stateMu.Unlock()
		if vis {
			t.Fatal("expected OnWindowBlur() to hide window when shouldHideOnBlur() is true")
		}
	})

	t.Run("dragging_mouse_down_does_not_hide", func(t *testing.T) {
		app := newBlurTestApp(t)
		app.isMouseDownFn = func() bool { return true }
		if app.shouldHideOnBlur() {
			t.Fatal("expected shouldHideOnBlur() to return false while mouse button is down")
		}
		app.OnWindowBlur()
		app.stateMu.Lock()
		vis := app.visible
		app.stateMu.Unlock()
		if !vis {
			t.Fatal("expected OnWindowBlur() not to hide while mouse button is down")
		}
	})

	t.Run("process_foreground_does_not_hide", func(t *testing.T) {
		app := newBlurTestApp(t)
		app.isProcessFgFn = func() bool { return true }
		if app.shouldHideOnBlur() {
			t.Fatal("expected shouldHideOnBlur() to return false while process is foreground")
		}
	})

	t.Run("cursor_inside_window_does_not_hide", func(t *testing.T) {
		app := newBlurTestApp(t)
		app.isCursorInWinFn = func(uintptr) bool { return true }
		if app.shouldHideOnBlur() {
			t.Fatal("expected shouldHideOnBlur() to return false while cursor is inside window")
		}
	})

	t.Run("hide_on_blur_disabled_does_not_hide", func(t *testing.T) {
		app := newBlurTestApp(t)
		cfg := app.GetConfig()
		cfg.Settings.HideOnBlur = false
		if err := app.SaveSettings(cfg.Settings); err != nil {
			t.Fatalf("SaveSettings failed: %v", err)
		}
		if app.shouldHideOnBlur() {
			t.Fatal("expected shouldHideOnBlur() to return false when HideOnBlur is false")
		}
	})

	t.Run("dialog_open_does_not_hide", func(t *testing.T) {
		for _, open := range []func(){
			func() {
				app := newBlurTestApp(t)
				app.SetModalOpen(true)
				if app.shouldHideOnBlur() {
					t.Fatal("expected false while a UI modal is open")
				}
			},
			func() {
				app := newBlurTestApp(t)
				app.nativeDialogOpen.Store(true)
				if app.shouldHideOnBlur() {
					t.Fatal("expected false while a native dialog is open")
				}
			},
		} {
			open()
		}
	})

	t.Run("closing_a_native_dialog_keeps_the_ui_modal", func(t *testing.T) {
		app := newBlurTestApp(t)
		app.SetModalOpen(true)
		app.nativeDialogOpen.Store(true)
		app.nativeDialogOpen.Store(false)
		if !app.modalOpen() {
			t.Fatal("closing a picker must not clear the open UI modal")
		}
	})

	t.Run("within_grace_period_does_not_hide", func(t *testing.T) {
		app := newBlurTestApp(t)
		app.stateMu.Lock()
		app.lastShown = time.Now()
		app.stateMu.Unlock()
		if app.shouldHideOnBlur() {
			t.Fatal("expected shouldHideOnBlur() to return false within grace period")
		}
	})

	t.Run("not_visible_does_not_hide", func(t *testing.T) {
		app := newBlurTestApp(t)
		app.isVisibleFn = func(uintptr) bool { return false }
		if app.shouldHideOnBlur() {
			t.Fatal("expected shouldHideOnBlur() to return false when window is not visible")
		}
	})
}

func TestToggleWindowVisibilityAndForeground(t *testing.T) {
	app := newBlurTestApp(t)

	// 1. Hidden -> ToggleWindow shows window.
	app.stateMu.Lock()
	app.visible = false
	app.stateMu.Unlock()
	app.isVisibleFn = func(uintptr) bool { return false }

	app.ToggleWindow()
	app.stateMu.Lock()
	vis := app.visible
	app.stateMu.Unlock()
	if !vis {
		t.Fatal("expected ToggleWindow() to show hidden window")
	}

	// 2. Visible but buried behind another window -> ToggleWindow brings to front instead of hiding.
	oldShown := time.Now().Add(-time.Minute)
	app.stateMu.Lock()
	app.visible = true
	app.lastShown = oldShown
	app.stateMu.Unlock()
	app.isVisibleFn = func(uintptr) bool { return true }
	app.isProcessFgFn = func() bool { return false }

	app.ToggleWindow()
	app.stateMu.Lock()
	vis = app.visible
	newShown := app.lastShown
	app.stateMu.Unlock()
	if !vis {
		t.Fatal("expected ToggleWindow() to keep buried visible window shown (bring to front)")
	}
	if !newShown.After(oldShown) {
		t.Fatal("expected ToggleWindow() on buried visible window to refresh lastShown via ShowWindow()")
	}

	// 3. Visible and foreground -> ToggleWindow hides window.
	app.isProcessFgFn = func() bool { return true }
	app.ToggleWindow()
	app.stateMu.Lock()
	vis = app.visible
	app.stateMu.Unlock()
	if vis {
		t.Fatal("expected ToggleWindow() to hide foreground visible window")
	}
}

func TestHideOnEscDefaultAndPersistence(t *testing.T) {
	app := newTestApp(t)

	initial := app.GetConfig().Settings
	if !initial.HideOnEsc {
		t.Fatal("expected HideOnEsc to default to true")
	}

	updated := initial
	updated.HideOnEsc = false
	if err := app.SaveSettings(updated); err != nil {
		t.Fatalf("SaveSettings(HideOnEsc=false) failed: %v", err)
	}
	if app.GetConfig().Settings.HideOnEsc {
		t.Fatal("expected in-memory HideOnEsc to be false after SaveSettings")
	}

	reloaded, err := config.Load(app.configPath)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}
	if reloaded.Settings.HideOnEsc {
		t.Fatal("expected persisted HideOnEsc to be false on disk")
	}

	updated.HideOnEsc = true
	if err := app.SaveSettings(updated); err != nil {
		t.Fatalf("SaveSettings(HideOnEsc=true) failed: %v", err)
	}
	reloaded, err = config.Load(app.configPath)
	if err != nil {
		t.Fatalf("config.Load failed: %v", err)
	}
	if !reloaded.Settings.HideOnEsc {
		t.Fatal("expected persisted HideOnEsc to be true on disk")
	}
}

func TestSingleInstanceLockAndWake(t *testing.T) {
	suffix := fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
	mutexName := "Local\\JeeDesk-Test-Mutex-" + suffix
	className := "JeeDesk-Test-Class-" + suffix
	windowName := "JeeDesk-Test-Window-" + suffix

	wakeCh := make(chan struct{}, 1)
	primary, isPrimary, err := AcquireSingleInstance(mutexName, className, windowName, func() {
		select {
		case wakeCh <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatalf("first AcquireSingleInstance failed: %v", err)
	}
	if !isPrimary || primary == nil {
		t.Fatalf("expected first AcquireSingleInstance to be primary, got isPrimary=%v primary=%v", isPrimary, primary)
	}
	t.Cleanup(func() {
		primary.Close()
	})

	second, isSecondPrimary, err := AcquireSingleInstance(mutexName, className, windowName, nil)
	if err != nil {
		t.Fatalf("second AcquireSingleInstance failed: %v", err)
	}
	if isSecondPrimary || second != nil {
		if second != nil {
			second.Close()
		}
		t.Fatalf("expected second AcquireSingleInstance not to be primary, got isPrimary=%v", isSecondPrimary)
	}

	select {
	case <-wakeCh:
	case <-time.After(1 * time.Second):
		t.Fatal("expected primary instance onWake callback to fire within 1 second")
	}

	primary.Close()
	primary.Close()

	third, isThirdPrimary, err := AcquireSingleInstance(mutexName, className, windowName, nil)
	if err != nil {
		t.Fatalf("third AcquireSingleInstance failed: %v", err)
	}
	if !isThirdPrimary || third == nil {
		t.Fatalf("expected third AcquireSingleInstance after Close() to be primary, got isPrimary=%v", isThirdPrimary)
	}
	third.Close()
}

func TestAppWritesRefusedWhileConfigBroken(t *testing.T) {
	app := newTestApp(t)

	broken := []byte(`{"settings": { "hotkey": "Ctrl+K"`)
	if err := os.WriteFile(app.configPath, broken, 0o644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}
	if _, err := app.configManager().Reload(); err == nil {
		t.Fatal("expected Reload to fail on broken config")
	}

	// A change made through the app must be refused and leave the user's file as it is.
	_, err := app.AddGroup("不会写入")
	if !errors.Is(err, config.ErrWriteBlocked) {
		t.Fatalf("expected ErrWriteBlocked, got %v", err)
	}
	got, err := os.ReadFile(app.configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !bytes.Equal(got, broken) {
		t.Fatalf("broken config.json was overwritten:\n%s", got)
	}

	// Once the file is fixed and reloaded, changes go through again.
	if err := config.Save(app.configPath, config.DefaultConfig()); err != nil {
		t.Fatalf("repair config: %v", err)
	}
	if _, err := app.configManager().Reload(); err != nil {
		t.Fatalf("reload repaired config: %v", err)
	}
	if _, err := app.AddGroup("修复后写入"); err != nil {
		t.Fatalf("AddGroup after repair failed: %v", err)
	}
	onDisk, err := config.Load(app.configPath)
	if err != nil {
		t.Fatalf("Load after AddGroup failed: %v", err)
	}
	if len(onDisk.Groups) != 2 {
		t.Fatalf("expected 2 groups on disk after repair, got %d", len(onDisk.Groups))
	}
}

func TestPickBaseDirSkipsForeignConfigInWorkingDir(t *testing.T) {
	cwd := t.TempDir()
	// Only the path matters here, so the executable does not have to exist. It is kept out of the
	// temp directory so that it does not look like a go run binary.
	exeDir := filepath.Join(filepath.VolumeName(cwd)+`\`, "JeeDesk")
	exePath := filepath.Join(exeDir, "JeeDesk.exe")
	cfgPath := filepath.Join(cwd, "config.json")

	// A config.json that parses as JSON but has none of JeeDesk's keys belongs to another program.
	if err := os.WriteFile(cfgPath, []byte(`{"name":"other-app","scripts":{}}`), 0o644); err != nil {
		t.Fatalf("write foreign config: %v", err)
	}
	if got := pickBaseDir(cwd, true, exePath, true); got != exeDir {
		t.Fatalf("foreign config.json was adopted: got %q, want %q", got, exeDir)
	}

	// JeeDesk's own config is adopted.
	if err := os.WriteFile(cfgPath, []byte(`{"settings":{"hotkey":"Alt+K"},"groups":[]}`), 0o644); err != nil {
		t.Fatalf("write jeedesk config: %v", err)
	}
	if got := pickBaseDir(cwd, true, exePath, true); got != cwd {
		t.Fatalf("JeeDesk config.json was not adopted: got %q, want %q", got, cwd)
	}

	// A config that no longer parses may still be JeeDesk's, so it is adopted and reported as broken.
	if err := os.WriteFile(cfgPath, []byte(`{"settings": { broken`), 0o644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}
	if got := pickBaseDir(cwd, true, exePath, true); got != cwd {
		t.Fatalf("broken config.json was not adopted: got %q, want %q", got, cwd)
	}

	// Without a config.json there, the executable's directory is used.
	if err := os.Remove(cfgPath); err != nil {
		t.Fatalf("remove config: %v", err)
	}
	if got := pickBaseDir(cwd, true, exePath, true); got != exeDir {
		t.Fatalf("expected executable dir without config.json: got %q, want %q", got, exeDir)
	}
}

func TestIsDevBinaryDir(t *testing.T) {
	project := `F:\Codes\JeeDesk`
	cases := []struct {
		name   string
		exeDir string
		cwd    string
		want   bool
	}{
		{"go run build cache", `C:\Users\u\AppData\Local\Temp\go-build123\b001\exe`, project, true},
		{"go-build outside the default temp", `D:\tmp\go-build456\b001\exe`, project, true},
		{"wails dev output", `F:\Codes\JeeDesk\build\bin`, project, true},
		{"wails dev output, other case", `f:\codes\jeedesk\Build\Bin`, project, true},
		{"packaged binary", `C:\Tools\JeeDesk`, project, false},
		{"another project's build/bin", `F:\Codes\Other\build\bin`, project, false},
		{"build/bin started with itself as working directory", `F:\Codes\JeeDesk\build\bin`, `F:\Codes\JeeDesk\build\bin`, false},
		{"unknown working directory", `C:\Tools\JeeDesk`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDevBinaryDir(tc.exeDir, tc.cwd); got != tc.want {
				t.Fatalf("isDevBinaryDir(%q, %q) = %v, want %v", tc.exeDir, tc.cwd, got, tc.want)
			}
		})
	}
}

func TestPickBaseDirUsesWorkingDirInDev(t *testing.T) {
	// A path outside the temp directory, so only the build/bin rule can mark it as development.
	volume := filepath.VolumeName(t.TempDir()) + `\`
	cwd := filepath.Join(volume, "does-not-exist", "project")

	// wails dev keeps the working directory and builds into <project>/build/bin.
	if got := pickBaseDir(cwd, true, filepath.Join(cwd, "build", "bin", "JeeDesk.exe"), true); got != cwd {
		t.Fatalf("wails dev: got %q, want working directory %q", got, cwd)
	}

	// Started from Explorer instead, the same binary runs with build/bin as working directory.
	binDir := filepath.Join(cwd, "build", "bin")
	if got := pickBaseDir(binDir, true, filepath.Join(binDir, "JeeDesk.exe"), true); got != binDir {
		t.Fatalf("explorer launch: got %q, want executable directory %q", got, binDir)
	}
}

// Hotkeys for the app-level hotkey tests. The hook package's own tests use Ctrl+Alt+Shift+F20 to
// F23 and run in a separate process, so these stay clear of them.
const (
	testHotkeyFree = "Ctrl+Alt+Shift+F19"
	testHotkeyHeld = "Ctrl+Alt+Shift+F24"
)

func newHotkeyTestApp(t *testing.T) (*App, *hook.HotkeyManager, *[]string) {
	t.Helper()
	app := newTestApp(t)
	mgr := hook.NewHotkeyManager(func() {})
	t.Cleanup(mgr.Stop)
	app.hotkeyMgr = mgr

	toasts := &[]string{}
	app.toastFunc = func(_, message string) { *toasts = append(*toasts, message) }
	return app, mgr, toasts
}

// holdHotkey registers spec from a second manager, the way another program would occupy it.
func holdHotkey(t *testing.T, spec string) *hook.HotkeyManager {
	t.Helper()
	holder := hook.NewHotkeyManager(func() {})
	if err := holder.Start(spec); err != nil {
		holder.Stop()
		t.Skipf("cannot register %s on this machine: %v", spec, err)
	}
	t.Cleanup(holder.Stop)
	return holder
}

func TestHotkeyStatusFollowsActualRegistration(t *testing.T) {
	app, mgr, toasts := newHotkeyTestApp(t)
	holdHotkey(t, testHotkeyHeld)

	if err := mgr.Start(testHotkeyFree); err != nil {
		t.Skipf("cannot register %s on this machine: %v", testHotkeyFree, err)
	}

	// Switching to a hotkey another program holds fails and keeps the current one.
	app.applyHotkey(testHotkeyHeld, true)
	if st := app.GetHotkeyStatus(); st.Registered || st.Hotkey != testHotkeyHeld {
		t.Fatalf("expected %s to be reported as not registered, got %+v", testHotkeyHeld, st)
	}
	if got := app.appliedHotkey; got != testHotkeyFree {
		t.Fatalf("expected %s to stay applied, got %q", testHotkeyFree, got)
	}

	// Going back to the hotkey that is still held only has to refresh the reported status.
	app.syncHooks(config.Settings{Hotkey: testHotkeyFree}, true)
	st := app.GetHotkeyStatus()
	if !st.Registered || st.Hotkey != testHotkeyFree || st.Error != "" {
		t.Fatalf("expected %s to be reported as registered, got %+v", testHotkeyFree, st)
	}

	// A config reload that echoes the same failing change does not repeat the message.
	app.applyHotkey(testHotkeyHeld, true)
	app.applyHotkey(testHotkeyHeld, false)
	if n := len(*toasts); n != 2 {
		t.Fatalf("expected %d toasts, got %d: %q", 2, n, *toasts)
	}
}

func TestSaveSettingsRejectsHotkeyThatCannotRegister(t *testing.T) {
	app, _, _ := newHotkeyTestApp(t)
	holder := holdHotkey(t, testHotkeyHeld)

	// A hotkey another program holds must not be written: the dialog stays open and the file
	// keeps the working hotkey, which is what the user still has.
	before := app.GetConfig().Settings.Hotkey
	err := app.SaveSettings(config.Settings{Hotkey: testHotkeyHeld, MouseTrigger: "none"})
	if err == nil {
		t.Fatalf("expected SaveSettings to fail for the held hotkey")
	}
	if got := app.GetConfig().Settings.Hotkey; got != before {
		t.Fatalf("stored hotkey = %q, want unchanged %q", got, before)
	}
	if st := app.GetHotkeyStatus(); st.Hotkey != before {
		t.Fatalf("expected the reported hotkey to stay %s, got %+v", before, st)
	}

	// Once the key is released the same save works and is persisted.
	holder.Stop()
	if err := app.SaveSettings(config.Settings{Hotkey: testHotkeyHeld, MouseTrigger: "none"}); err != nil {
		t.Fatalf("SaveSettings after release failed: %v", err)
	}
	if got := app.GetConfig().Settings.Hotkey; got != testHotkeyHeld {
		t.Fatalf("stored hotkey = %q, want %q", got, testHotkeyHeld)
	}
	if st := app.GetHotkeyStatus(); !st.Registered || st.Hotkey != testHotkeyHeld {
		t.Fatalf("expected %s to be registered, got %+v", testHotkeyHeld, st)
	}
}

func TestContextAccessIsSynchronised(t *testing.T) {
	app := newTestApp(t)
	if got := app.context(); got != nil {
		t.Fatalf("context before startup = %v, want nil", got)
	}

	// Wails calls OnStartup on its own goroutine while the single-instance wake callback may
	// already be reading the context. Run both against each other; the race detector judges the pairing.
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			app.setContext(ctx)
		}()
		go func() {
			defer wg.Done()
			_ = app.context()
		}()
	}
	wg.Wait()

	if got := app.context(); got != ctx {
		t.Fatalf("context after startup = %v, want %v", got, ctx)
	}
}

func TestLoadConfigKeepsManagerUsableWhenConfigBroken(t *testing.T) {
	dir := t.TempDir()
	broken := []byte(`{"settings": { broken`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), broken, 0o644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}

	// This is the startup sequence from main: the config is loaded before the app runs.
	app := NewApp(dir)
	app.loadConfig()
	t.Cleanup(func() { _ = app.configManager().Close() })

	if app.configManager() == nil {
		t.Fatal("expected a usable manager despite the load error")
	}
	if app.configLoadErr == nil {
		t.Fatal("expected the load error to be kept for startup to report")
	}

	// The manager startup relies on is the one that refuses to write over the broken file.
	if _, err := app.AddGroup("不会写入"); !errors.Is(err, config.ErrWriteBlocked) {
		t.Fatalf("expected ErrWriteBlocked, got %v", err)
	}
	got, err := os.ReadFile(app.configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !bytes.Equal(got, broken) {
		t.Fatalf("broken config.json was overwritten:\n%s", got)
	}
}
