package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultHotkey       = "Alt+G"
	DefaultMouseTrigger = "none"

	// ClickModeDouble launches an item on double click, ClickModeSingle on a single click.
	ClickModeDouble = "double"
	ClickModeSingle = "single"

	ThemeDark   = "dark"
	ThemeLight  = "light"
	ThemeSystem = "system"

	LanguageChinese = "zh"
	LanguageEnglish = "en"

	DefaultWindowWidth  = 960
	DefaultWindowHeight = 640

	MinWindowOpacity = 50
	MinFontScale     = 80
	MaxFontScale     = 140
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Config represents the root application configuration persisted in config.json.
type Config struct {
	Settings Settings `json:"settings"`
	Groups   []Group  `json:"groups"`

	// Warnings collects problems Normalize found while repairing values read from disk. They are
	// never serialized; the application shows them so a typo in config.json is not silent.
	Warnings []string `json:"-"`
}

// Settings holds global hotkey, mouse trigger, launch, and window options.
type Settings struct {
	Hotkey       string `json:"hotkey"`
	MouseTrigger string `json:"mouse_trigger"`
	// MouseSwallow hides the trigger mouse button from other applications while the
	// trigger is a mouse button.
	MouseSwallow bool `json:"mouse_swallow"`

	HideOnLaunch bool `json:"hide_on_launch"`
	HideOnBlur   bool `json:"hide_on_blur"`
	HideOnEsc    bool `json:"hide_on_esc"`
	// StrictBlur hides the window even while the cursor rests over it. Without it, leaving the
	// cursor on the panel keeps the window open after another application takes the foreground.
	StrictBlur bool `json:"hide_on_blur_strict"`

	// ClickMode decides whether a single or a double click launches an item.
	ClickMode string `json:"click_mode"`

	// AutoStart registers the executable under HKCU Run for the current user.
	AutoStart bool `json:"auto_start"`
	// StartHidden keeps the window closed on startup; the hotkey still summons it.
	StartHidden bool `json:"start_hidden"`

	Theme  string `json:"theme"`
	Lang   string `json:"language"`
	Recent bool   `json:"show_recent"`

	// CollapsedGroups remembers which sidebar groups the user folded away.
	CollapsedGroups []string `json:"collapsed_groups"`

	// WindowOpacity is a percentage, WindowWidth and WindowHeight are device independent
	// pixels with 0 meaning the default size. WindowX and WindowY are the last window
	// position, with 0,0 meaning "let the application choose".
	// WindowPositioned marks a saved position as real: monitors left of or above the primary one
	// have negative coordinates, so 0,0 cannot mean "unset" on its own.
	WindowPositioned bool `json:"window_positioned"`
	WindowOpacity    int  `json:"window_opacity"`
	WindowWidth      int  `json:"window_width"`
	WindowHeight     int  `json:"window_height"`
	WindowX          int  `json:"window_x"`
	WindowY          int  `json:"window_y"`
	FontScale        int  `json:"font_scale"`
}

// Group represents a named category of launcher items.
type Group struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Items []Item `json:"items"`
}

// Item represents a launchable application, shortcut, script, or folder.
type Item struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Path    string `json:"path"`
	Args    string `json:"args"`
	WorkDir string `json:"work_dir"`
	Icon    string `json:"icon"`
	// Admin launches the target elevated through UAC.
	Admin bool `json:"admin"`
}

// DefaultConfig returns a fresh default configuration.
func DefaultConfig() Config {
	return Config{
		Settings: Settings{
			Hotkey:        DefaultHotkey,
			MouseTrigger:  DefaultMouseTrigger,
			MouseSwallow:  true,
			HideOnLaunch:  true,
			HideOnBlur:    true,
			HideOnEsc:     true,
			ClickMode:     ClickModeDouble,
			Theme:         ThemeDark,
			Lang:          LanguageChinese,
			Recent:        true,
			WindowOpacity: 100,
			FontScale:     100,
		},
		Groups: []Group{
			{
				ID:    "default",
				Name:  "常用工具",
				Items: []Item{},
			},
		},
	}
}

// Normalize ensures slices are non-nil and validates/defaults settings fields.
func Normalize(cfg *Config) {
	if cfg == nil {
		return
	}
	cfg.Warnings = nil

	if strings.TrimSpace(cfg.Settings.Hotkey) == "" {
		cfg.Settings.Hotkey = DefaultHotkey
	}

	rawMouseTrigger := strings.TrimSpace(cfg.Settings.MouseTrigger)
	switch strings.ToLower(rawMouseTrigger) {
	case "none":
		cfg.Settings.MouseTrigger = "none"
	case "middle":
		cfg.Settings.MouseTrigger = "middle"
	case "xbutton1":
		cfg.Settings.MouseTrigger = "xbutton1"
	case "xbutton2":
		cfg.Settings.MouseTrigger = "xbutton2"
	default:
		cfg.Settings.MouseTrigger = DefaultMouseTrigger
		if rawMouseTrigger != "" {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("鼠标呼出设置 %q 无法识别，已回退到 none", rawMouseTrigger))
		}
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Settings.ClickMode)) {
	case ClickModeSingle:
		cfg.Settings.ClickMode = ClickModeSingle
	default:
		cfg.Settings.ClickMode = ClickModeDouble
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Settings.Theme)) {
	case ThemeLight:
		cfg.Settings.Theme = ThemeLight
	case ThemeSystem:
		cfg.Settings.Theme = ThemeSystem
	default:
		cfg.Settings.Theme = ThemeDark
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Settings.Lang)) {
	case LanguageEnglish:
		cfg.Settings.Lang = LanguageEnglish
	default:
		cfg.Settings.Lang = LanguageChinese
	}

	if cfg.Settings.WindowOpacity == 0 {
		cfg.Settings.WindowOpacity = 100
	}
	if cfg.Settings.WindowOpacity < MinWindowOpacity {
		cfg.Settings.WindowOpacity = MinWindowOpacity
	}
	if cfg.Settings.WindowOpacity > 100 {
		cfg.Settings.WindowOpacity = 100
	}

	if cfg.Settings.FontScale == 0 {
		cfg.Settings.FontScale = 100
	}
	if cfg.Settings.FontScale < MinFontScale {
		cfg.Settings.FontScale = MinFontScale
	}
	if cfg.Settings.FontScale > MaxFontScale {
		cfg.Settings.FontScale = MaxFontScale
	}

	if cfg.Settings.WindowWidth <= 0 {
		cfg.Settings.WindowWidth = 0
	}
	if cfg.Settings.WindowHeight <= 0 {
		cfg.Settings.WindowHeight = 0
	}

	if cfg.Settings.CollapsedGroups == nil {
		cfg.Settings.CollapsedGroups = []string{}
	}

	if cfg.Groups == nil {
		cfg.Groups = []Group{}
	}
	for i := range cfg.Groups {
		if cfg.Groups[i].Items == nil {
			cfg.Groups[i].Items = []Item{}
		}
	}
}

// Clone returns a deep copy of cfg including all groups and items slices.
func Clone(cfg Config) Config {
	out := cfg
	if cfg.Groups != nil {
		out.Groups = make([]Group, len(cfg.Groups))
		for i, g := range cfg.Groups {
			out.Groups[i] = g
			if g.Items != nil {
				out.Groups[i].Items = make([]Item, len(g.Items))
				copy(out.Groups[i].Items, g.Items)
			}
		}
	}
	return out
}

// Load reads, BOM-strips, parses, and normalizes the config file at path.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	return parse(data, path)
}

func parse(data []byte, path string) (Config, error) {
	data = stripBOM(data)

	cfg := Config{
		Settings: DefaultConfig().Settings,
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	Normalize(&cfg)
	return cfg, nil
}

func stripBOM(data []byte) []byte {
	return bytes.TrimPrefix(data, utf8BOM)
}

// isBlank reports whether config file contents hold nothing but whitespace after an optional BOM.
func isBlank(data []byte) bool {
	return len(bytes.TrimSpace(stripBOM(data))) == 0
}

// LoadOrCreate loads the config at path. A missing or blank file carries nothing to load and is
// created with DefaultConfig; any other existing file is loaded as is, even when it fails to parse.
func LoadOrCreate(path string) (Config, error) {
	data, err := os.ReadFile(path)
	switch {
	case err != nil && !errors.Is(err, os.ErrNotExist):
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	case err == nil && !isBlank(data):
		return parse(data, path)
	}

	cfg := DefaultConfig()
	if saveErr := Save(path, cfg); saveErr != nil {
		return Config{}, saveErr
	}
	return cfg, nil
}

// LooksLikeConfig reports whether the file at path can be treated as this application's config.
// A file that is not valid JSON may be a damaged config and is accepted: the Manager refuses to
// overwrite it until it parses again. Valid JSON without any of the config's top-level keys
// belongs to some other program and is rejected.
func LooksLikeConfig(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	data = stripBOM(data)
	if !json.Valid(data) {
		return true
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return false
	}
	_, hasSettings := top["settings"]
	_, hasGroups := top["groups"]
	return hasSettings || hasGroups
}

// BackupPath returns the path of the single rolling backup kept next to path.
func BackupPath(path string) string {
	return path + ".bak"
}

// Save normalizes a copy of cfg and writes it atomically to path, keeping the previous contents as
// path + ".bak" so a damaged file can be recovered by hand.
func Save(path string, cfg Config) error {
	return SaveWithBackup(path, cfg, true)
}

// SaveWithBackup writes cfg like Save, but only rotates the backup when backup is true. Callers
// that persist high-frequency, non-structural changes (window geometry) keep the last real version
// instead of overwriting it with a copy of itself.
func SaveWithBackup(path string, cfg Config, backup bool) error {
	next := Clone(cfg)
	Normalize(&next)

	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config directory %q: %w", dir, err)
	}

	if backup {
		backupExisting(path)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp config file: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace config %q: %w", path, err)
	}
	cleanup = false
	return nil
}

// backupExisting copies the current config file to path + ".bak". It is best effort: a failed
// backup must never block saving the new configuration.
func backupExisting(path string) {
	data, err := os.ReadFile(path)
	if err != nil || isBlank(data) {
		return
	}
	_ = os.WriteFile(BackupPath(path), data, 0o644)
}
