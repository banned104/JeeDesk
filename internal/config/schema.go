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

	data = bytes.TrimPrefix(data, utf8BOM)

	cfg := Config{
		Settings: DefaultConfig().Settings,
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	Normalize(&cfg)
	return cfg, nil
}

// LoadOrCreate loads the config at path, or creates it with DefaultConfig if missing.
func LoadOrCreate(path string) (Config, error) {
	_, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := DefaultConfig()
			if saveErr := Save(path, cfg); saveErr != nil {
				return Config{}, saveErr
			}
			return cfg, nil
		}
		return Config{}, fmt.Errorf("stat config %q: %w", path, err)
	}

	return Load(path)
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
