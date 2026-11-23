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
)

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// Config represents the root application configuration persisted in config.json.
type Config struct {
	Settings Settings `json:"settings"`
	Groups   []Group  `json:"groups"`
}

// Settings holds global hotkey, mouse trigger, and window visibility options.
type Settings struct {
	Hotkey       string `json:"hotkey"`
	MouseTrigger string `json:"mouse_trigger"`
	HideOnLaunch bool   `json:"hide_on_launch"`
	HideOnBlur   bool   `json:"hide_on_blur"`
	HideOnEsc    bool   `json:"hide_on_esc"`
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
}

// DefaultConfig returns a fresh default configuration.
func DefaultConfig() Config {
	return Config{
		Settings: Settings{
			Hotkey:       DefaultHotkey,
			MouseTrigger: DefaultMouseTrigger,
			HideOnLaunch: true,
			HideOnBlur:   true,
			HideOnEsc:    true,
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

	if strings.TrimSpace(cfg.Settings.Hotkey) == "" {
		cfg.Settings.Hotkey = DefaultHotkey
	}

	switch strings.ToLower(strings.TrimSpace(cfg.Settings.MouseTrigger)) {
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

// Save normalizes a copy of cfg and writes it atomically to path.
func Save(path string, cfg Config) error {
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
