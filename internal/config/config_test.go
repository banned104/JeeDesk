package config

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoadOrCreate(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "nested", "dir", "config.json")

	cfg, err := LoadOrCreate(cfgPath)
	if err != nil {
		t.Fatalf("LoadOrCreate failed: %v", err)
	}

	if cfg.Settings.Hotkey != "Alt+G" {
		t.Fatalf("expected default hotkey Alt+G, got %q", cfg.Settings.Hotkey)
	}
	if cfg.Settings.MouseTrigger != "none" {
		t.Fatalf("expected default mouse_trigger none, got %q", cfg.Settings.MouseTrigger)
	}
	if !cfg.Settings.HideOnLaunch || !cfg.Settings.HideOnBlur {
		t.Fatalf("expected HideOnLaunch and HideOnBlur true by default, got %+v", cfg.Settings)
	}
	if len(cfg.Groups) != 1 || cfg.Groups[0].ID != "default" || cfg.Groups[0].Name != "常用工具" || cfg.Groups[0].Items == nil {
		t.Fatalf("unexpected default groups: %+v", cfg.Groups)
	}

	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("expected config file to be created: %v", err)
	}

	// Modify file and verify LoadOrCreate loads existing file instead of overwriting.
	custom := cfg
	custom.Settings.Hotkey = "Ctrl+Space"
	if err := Save(cfgPath, custom); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := LoadOrCreate(cfgPath)
	if err != nil {
		t.Fatalf("second LoadOrCreate failed: %v", err)
	}
	if loaded.Settings.Hotkey != "Ctrl+Space" {
		t.Fatalf("expected existing hotkey Ctrl+Space, got %q", loaded.Settings.Hotkey)
	}
}

func TestLoadUTF8BOM(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	rawJSON := `{"settings":{"hotkey":"Alt+Space","mouse_trigger":"XButton1","hide_on_launch":false,"hide_on_blur":true},"groups":[{"id":"g1","name":"Work","items":null}]}`
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte(rawJSON)...)
	if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load with UTF-8 BOM failed: %v", err)
	}

	if cfg.Settings.Hotkey != "Alt+Space" {
		t.Fatalf("expected hotkey Alt+Space, got %q", cfg.Settings.Hotkey)
	}
	if cfg.Settings.MouseTrigger != "xbutton1" {
		t.Fatalf("expected normalized mouse_trigger xbutton1, got %q", cfg.Settings.MouseTrigger)
	}
	if cfg.Settings.HideOnLaunch {
		t.Fatalf("expected hide_on_launch false")
	}
	if len(cfg.Groups) != 1 || cfg.Groups[0].Items == nil {
		t.Fatalf("expected non-nil normalized items slice, got %+v", cfg.Groups)
	}
}

func TestSaveAndLoadRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	orig := Config{
		Settings: Settings{
			Hotkey:       "Ctrl+Alt+J",
			MouseTrigger: "Middle",
			HideOnLaunch: false,
			HideOnBlur:   true,
		},
		Groups: []Group{
			{
				ID:   "dev",
				Name: "Dev Tools",
				Items: []Item{
					{
						ID:      "item-1",
						Name:    "Notepad",
						Path:    `C:\Windows\System32\notepad.exe`,
						Args:    "readme.txt",
						WorkDir: `C:\Temp`,
						Icon:    "icons/abc.png",
					},
				},
			},
		},
	}

	if err := Save(cfgPath, orig); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.HasSuffix(raw, []byte("\n")) {
		t.Fatalf("expected trailing newline in saved JSON")
	}
	if !bytes.Contains(raw, []byte("\n  \"settings\": {")) {
		t.Fatalf("expected 2-space indent in saved JSON, got:\n%s", string(raw))
	}

	loaded, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Settings.MouseTrigger != "middle" {
		t.Fatalf("expected normalized mouse_trigger middle, got %q", loaded.Settings.MouseTrigger)
	}
	if len(loaded.Groups) != 1 || len(loaded.Groups[0].Items) != 1 {
		t.Fatalf("unexpected loaded groups: %+v", loaded.Groups)
	}
	if loaded.Groups[0].Items[0] != orig.Groups[0].Items[0] {
		t.Fatalf("item mismatch: got %+v, want %+v", loaded.Groups[0].Items[0], orig.Groups[0].Items[0])
	}

	// Verify Clone produces an independent deep copy.
	cloned := Clone(loaded)
	cloned.Groups[0].Items[0].Name = "Mutated"
	if loaded.Groups[0].Items[0].Name != "Notepad" {
		t.Fatalf("Clone mutated original slice element")
	}
}

func TestNormalize(t *testing.T) {
	cfg := Config{
		Settings: Settings{
			Hotkey:       "   ",
			MouseTrigger: "INVALID",
		},
		Groups: nil,
	}
	Normalize(&cfg)

	if cfg.Settings.Hotkey != "Alt+G" {
		t.Fatalf("expected empty hotkey to default to Alt+G, got %q", cfg.Settings.Hotkey)
	}
	if cfg.Settings.MouseTrigger != "none" {
		t.Fatalf("expected invalid mouse_trigger to reset to none, got %q", cfg.Settings.MouseTrigger)
	}
	if cfg.Groups == nil {
		t.Fatalf("expected non-nil Groups slice")
	}

	cases := []struct {
		in   string
		want string
	}{
		{"NONE", "none"},
		{"Middle", "middle"},
		{"XButton1", "xbutton1"},
		{"xbutton2", "xbutton2"},
		{"right", "none"},
		{"", "none"},
	}
	for _, tc := range cases {
		c := DefaultConfig()
		c.Settings.MouseTrigger = tc.in
		c.Groups[0].Items = nil
		Normalize(&c)
		if c.Settings.MouseTrigger != tc.want {
			t.Errorf("Normalize MouseTrigger(%q) = %q, want %q", tc.in, c.Settings.MouseTrigger, tc.want)
		}
		if c.Groups[0].Items == nil {
			t.Errorf("expected non-nil Items slice after Normalize")
		}
	}
}

func TestManagerUpdate(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	mgr, err := NewManager(cfgPath, nil, nil)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer func() { _ = mgr.Close() }()

	if mgr.Path() != cfgPath {
		t.Fatalf("expected Path %q, got %q", cfgPath, mgr.Path())
	}

	// Error inside mutate must abort save and preserve state.
	sentinel := errors.New("mutate aborted")
	err = mgr.Update(func(c *Config) error {
		c.Settings.Hotkey = "F12"
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if mgr.Get().Settings.Hotkey != "Alt+G" {
		t.Fatalf("expected hotkey unchanged after aborted Update, got %q", mgr.Get().Settings.Hotkey)
	}

	// Successful update persists and normalizes.
	err = mgr.Update(func(c *Config) error {
		c.Settings.Hotkey = "Ctrl+Shift+Space"
		c.Settings.MouseTrigger = "XBUTTON2"
		c.Groups[0].Items = append(c.Groups[0].Items, Item{
			ID:   "calc",
			Name: "Calculator",
			Path: "calc.exe",
		})
		return nil
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got := mgr.Get()
	if got.Settings.Hotkey != "Ctrl+Shift+Space" || got.Settings.MouseTrigger != "xbutton2" {
		t.Fatalf("unexpected updated settings: %+v", got.Settings)
	}
	if len(got.Groups[0].Items) != 1 || got.Groups[0].Items[0].ID != "calc" {
		t.Fatalf("unexpected updated items: %+v", got.Groups[0].Items)
	}

	onDisk, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load after Update failed: %v", err)
	}
	if onDisk.Settings.Hotkey != "Ctrl+Shift+Space" || len(onDisk.Groups[0].Items) != 1 {
		t.Fatalf("disk config mismatch after Update: %+v", onDisk)
	}
}

func TestStartWatchingDebounceAndInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	var mu sync.Mutex
	var updates []Config
	var errs []error
	updateCh := make(chan Config, 16)
	errCh := make(chan error, 16)

	mgr, err := NewManager(
		cfgPath,
		func(c Config) {
			mu.Lock()
			updates = append(updates, c)
			mu.Unlock()
			updateCh <- c
		},
		func(err error) {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
			errCh <- err
		},
	)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer func() {
		if err := mgr.Close(); err != nil {
			t.Errorf("Close failed: %v", err)
		}
		// Idempotent second Close.
		if err := mgr.Close(); err != nil {
			t.Errorf("second Close failed: %v", err)
		}
	}()

	if err := mgr.StartWatching(); err != nil {
		t.Fatalf("StartWatching failed: %v", err)
	}

	// Give fsnotify goroutine a brief moment to register the watch.
	time.Sleep(50 * time.Millisecond)

	// Perform 3 rapid external edits; 200ms debounce should coalesce them into 1 reload with final value.
	for _, hk := range []string{"Alt+1", "Alt+2", "Alt+3"} {
		c := DefaultConfig()
		c.Settings.Hotkey = hk
		if err := Save(cfgPath, c); err != nil {
			t.Fatalf("external Save(%s) failed: %v", hk, err)
		}
		time.Sleep(30 * time.Millisecond)
	}

	select {
	case got := <-updateCh:
		if got.Settings.Hotkey != "Alt+3" {
			t.Fatalf("expected debounced update with final hotkey Alt+3, got %q", got.Settings.Hotkey)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for debounced config update")
	}

	// Ensure no extra spurious update fired for the coalesced writes.
	select {
	case extra := <-updateCh:
		t.Fatalf("expected single debounced update, got extra update: %+v", extra)
	case <-time.After(350 * time.Millisecond):
	}

	if mgr.Get().Settings.Hotkey != "Alt+3" {
		t.Fatalf("expected in-memory hotkey Alt+3, got %q", mgr.Get().Settings.Hotkey)
	}

	// Write invalid JSON externally -> must invoke onError and preserve last valid config ("Alt+3").
	if err := os.WriteFile(cfgPath, []byte(`{"settings": { invalid json`), 0o644); err != nil {
		t.Fatalf("WriteFile invalid JSON failed: %v", err)
	}

	select {
	case gotErr := <-errCh:
		if gotErr == nil {
			t.Fatalf("expected non-nil error from onError")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for onError on invalid JSON")
	}

	if mgr.Get().Settings.Hotkey != "Alt+3" {
		t.Fatalf("expected last valid config Alt+3 preserved after invalid JSON, got %q", mgr.Get().Settings.Hotkey)
	}

	// Recover by writing valid JSON again -> must invoke onUpdate.
	recovered := DefaultConfig()
	recovered.Settings.Hotkey = "Alt+9"
	if err := Save(cfgPath, recovered); err != nil {
		t.Fatalf("Save recovered config failed: %v", err)
	}

	select {
	case got := <-updateCh:
		if got.Settings.Hotkey != "Alt+9" {
			t.Fatalf("expected recovered hotkey Alt+9, got %q", got.Settings.Hotkey)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for recovered config update")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(updates) < 2 {
		t.Fatalf("expected at least 2 updates, got %d", len(updates))
	}
	if len(errs) == 0 {
		t.Fatalf("expected at least 1 error recorded")
	}
}

func TestLoadOrCreateBlankFileReplacedWithDefault(t *testing.T) {
	blankContents := map[string][]byte{
		"empty":          nil,
		"whitespace":     []byte(" \r\n\t"),
		"bom only":       {0xEF, 0xBB, 0xBF},
		"bom and spaces": append([]byte{0xEF, 0xBB, 0xBF}, ' ', '\n'),
	}
	for name, content := range blankContents {
		t.Run(name, func(t *testing.T) {
			cfgPath := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(cfgPath, content, 0o644); err != nil {
				t.Fatalf("WriteFile failed: %v", err)
			}

			cfg, err := LoadOrCreate(cfgPath)
			if err != nil {
				t.Fatalf("LoadOrCreate failed: %v", err)
			}
			if cfg.Settings.Hotkey != DefaultHotkey {
				t.Fatalf("expected default hotkey, got %q", cfg.Settings.Hotkey)
			}
			if _, err := Load(cfgPath); err != nil {
				t.Fatalf("expected a valid default config on disk, got %v", err)
			}
		})
	}
}

func TestLoadOrCreateLeavesBrokenFileUntouched(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	broken := []byte(`{"settings": { "hotkey": "Ctrl+K"`)
	if err := os.WriteFile(cfgPath, broken, 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if _, err := LoadOrCreate(cfgPath); err == nil {
		t.Fatal("expected a parse error for broken config")
	}
	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(got, broken) {
		t.Fatalf("broken config was modified:\n%s", got)
	}
}

func TestLooksLikeConfig(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"jeedesk config", `{"settings":{"hotkey":"Alt+G"},"groups":[]}`, true},
		{"groups only", `{"groups":[{"id":"default","name":"常用工具","items":[]}]}`, true},
		{"broken json", `{"settings": { broken json`, true},
		{"blank", "   \n", true},
		{"with bom", "\xEF\xBB\xBF" + `{"settings":{"hotkey":"Alt+G"}}`, true},
		{"foreign object", `{"name":"other-app","version":2}`, false},
		{"empty object", `{}`, false},
		{"json array", `[1,2,3]`, false},
		{"foreign object with bom", "\xEF\xBB\xBF" + `{"name":"other-app"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfgPath := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(cfgPath, []byte(tc.content), 0o644); err != nil {
				t.Fatalf("WriteFile failed: %v", err)
			}
			if got := LooksLikeConfig(cfgPath); got != tc.want {
				t.Fatalf("LooksLikeConfig() = %v, want %v", got, tc.want)
			}
		})
	}

	if LooksLikeConfig(filepath.Join(t.TempDir(), "missing.json")) {
		t.Fatal("a missing file must not look like a config")
	}
}

func TestManagerBlocksWritesWhileConfigBroken(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	broken := []byte(`{"settings": { "hotkey": "Ctrl+K"`)
	if err := os.WriteFile(cfgPath, broken, 0o644); err != nil {
		t.Fatalf("WriteFile broken config failed: %v", err)
	}

	mgr, err := NewManagerWithFallback(cfgPath, nil, nil)
	if err == nil {
		t.Fatal("expected a load error for broken config")
	}
	defer func() { _ = mgr.Close() }()

	addGroup := func(c *Config) error {
		c.Groups = append(c.Groups, Group{ID: "g", Name: "G"})
		return nil
	}

	// A write must be refused and must leave both the file and memory untouched.
	err = mgr.Update(addGroup)
	if !errors.Is(err, ErrWriteBlocked) {
		t.Fatalf("expected ErrWriteBlocked, got %v", err)
	}
	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(got, broken) {
		t.Fatalf("broken config was overwritten:\n%s", got)
	}
	if n := len(mgr.Get().Groups); n != 1 {
		t.Fatalf("refused Update changed the in-memory config: %d groups", n)
	}

	// Repairing the file and reloading lifts the block.
	repaired := DefaultConfig()
	repaired.Settings.Hotkey = "Ctrl+K"
	if err := Save(cfgPath, repaired); err != nil {
		t.Fatalf("Save repaired config failed: %v", err)
	}
	if _, err := mgr.Reload(); err != nil {
		t.Fatalf("Reload after repair failed: %v", err)
	}
	if err := mgr.Update(addGroup); err != nil {
		t.Fatalf("Update after repair failed: %v", err)
	}
	onDisk, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load after Update failed: %v", err)
	}
	if len(onDisk.Groups) != 2 || onDisk.Settings.Hotkey != "Ctrl+K" {
		t.Fatalf("unexpected config after repair: %+v", onDisk)
	}
}

func TestManagerBlocksWritesAfterBrokenExternalEdit(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")

	mgr, err := NewManager(cfgPath, nil, nil)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer func() { _ = mgr.Close() }()

	broken := []byte(`{"settings": { "hotkey": "Ctrl+J"`)
	if err := os.WriteFile(cfgPath, broken, 0o644); err != nil {
		t.Fatalf("WriteFile broken config failed: %v", err)
	}
	if _, err := mgr.Reload(); err == nil {
		t.Fatal("expected Reload to fail on broken config")
	}

	// The last valid config stays in memory, but must not be saved over the user's edit.
	err = mgr.Update(func(c *Config) error {
		c.Groups = append(c.Groups, Group{ID: "g", Name: "G"})
		return nil
	})
	if !errors.Is(err, ErrWriteBlocked) {
		t.Fatalf("expected ErrWriteBlocked, got %v", err)
	}
	got, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if !bytes.Equal(got, broken) {
		t.Fatalf("user's edit was overwritten:\n%s", got)
	}
	if n := len(mgr.Get().Groups); n != 1 {
		t.Fatalf("in-memory config changed while blocked: %d groups", n)
	}
}

func TestNormalizeWarnsAboutUnknownMouseTrigger(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Settings.MouseTrigger = "side-button"
	Normalize(&cfg)

	if cfg.Settings.MouseTrigger != "none" {
		t.Fatalf("mouse trigger = %q, want none", cfg.Settings.MouseTrigger)
	}
	if len(cfg.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one entry", cfg.Warnings)
	}
	if !strings.Contains(cfg.Warnings[0], "side-button") {
		t.Fatalf("warning %q does not name the rejected value", cfg.Warnings[0])
	}

	// A clean config must not carry warnings, otherwise the UI would repeat an old message.
	clean := DefaultConfig()
	Normalize(&clean)
	if len(clean.Warnings) != 0 {
		t.Fatalf("clean config warnings = %v, want none", clean.Warnings)
	}
}

func TestCollapsedGroupsNormalizedAndPersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	cfg.Settings.CollapsedGroups = []string{"grp-a", "grp-b"}
	if err := Save(path, cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Settings.CollapsedGroups) != 2 || loaded.Settings.CollapsedGroups[1] != "grp-b" {
		t.Fatalf("collapsed groups = %v, want [grp-a grp-b]", loaded.Settings.CollapsedGroups)
	}
}
