package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"jeedesk/internal/config"
	"jeedesk/internal/tray"
)

func newTestApp(t *testing.T) (*App, *atomic.Int32) {
	t.Helper()
	dir := t.TempDir()
	app := NewApp(dir)

	var alertCount atomic.Int32
	app.alertFunc = func(_, _ string) {
		alertCount.Add(1)
	}

	mgr, err := config.NewManagerWithFallback(app.configPath, app.onConfigUpdated, app.onConfigError)
	if err != nil {
		t.Fatalf("NewManagerWithFallback failed: %v", err)
	}
	app.cfgMgr = mgr
	t.Cleanup(func() {
		_ = mgr.Close()
	})

	cfg := app.GetConfig()
	if len(cfg.Groups) > 0 {
		app.SetActiveGroup(cfg.Groups[0].ID)
	}
	return app, &alertCount
}

func TestAppGroupAndItemLifecycle(t *testing.T) {
	app, _ := newTestApp(t)

	if err := app.AddGroup("开发工具"); err != nil {
		t.Fatalf("AddGroup failed: %v", err)
	}

	cfg := app.GetConfig()
	if len(cfg.Groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(cfg.Groups))
	}
	devGroup := cfg.Groups[1]
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
	app, _ := newTestApp(t)

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
	app, alertCount := newTestApp(t)
	if err := app.cfgMgr.StartWatching(); err != nil {
		t.Fatalf("StartWatching failed: %v", err)
	}

	if err := app.AddGroup("KeepMe"); err != nil {
		t.Fatalf("AddGroup failed: %v", err)
	}

	// Corrupt config.json externally.
	if err := os.WriteFile(app.configPath, []byte(`{"settings": { broken json`), 0o644); err != nil {
		t.Fatalf("write broken json: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for alertCount.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if alertCount.Load() == 0 {
		t.Fatal("expected error alert to trigger on malformed config.json")
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
	app, _ := newTestApp(t)
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
		app := newBlurTestApp(t)
		app.dialogOpen.Store(true)
		if app.shouldHideOnBlur() {
			t.Fatal("expected shouldHideOnBlur() to return false when dialogOpen is true")
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
	app, _ := newTestApp(t)

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
