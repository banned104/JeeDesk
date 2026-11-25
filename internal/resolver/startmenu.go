package resolver

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// StartMenuEntry is one launchable shortcut found in the Windows Start Menu.
type StartMenuEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Args    string `json:"args"`
	WorkDir string `json:"work_dir"`
	LnkPath string `json:"lnk_path"`
	IconSrc string `json:"icon_src"`
	// Scope is "user" or "machine", which is what the shortcut's folder implies.
	Scope string `json:"scope"`
}

const (
	scopeUser    = "user"
	scopeMachine = "machine"
	// maxStartMenuDepth keeps the scan away from deeply nested helper folders.
	maxStartMenuDepth = 4
)

// StartMenuRoot is one Start Menu program folder and the scope its entries belong to.
type StartMenuRoot struct {
	Dir   string
	Scope string
}

// StartMenuDirs returns the existing Start Menu program folders, user first.
func StartMenuDirs() []StartMenuRoot {
	candidates := []StartMenuRoot{
		{filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`), scopeUser},
		{filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`), scopeMachine},
	}

	out := make([]StartMenuRoot, 0, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Dir) == "" {
			continue
		}
		if info, err := os.Stat(candidate.Dir); err == nil && info.IsDir() {
			out = append(out, candidate)
		}
	}
	return out
}

// ScanStartMenu lists .lnk shortcuts from the Start Menu, resolving each to its target.
// Shortcuts that resolve to nothing launchable, such as uninstallers or help links, are dropped.
// The same target reached from both scopes is reported once, preferring the user entry.
func ScanStartMenu() []StartMenuEntry {
	seen := make(map[string]bool)
	out := make([]StartMenuEntry, 0, 64)

	for _, root := range StartMenuDirs() {
		rootDir := root.Dir
		_ = filepath.WalkDir(rootDir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if depthOf(rootDir, path) > maxStartMenuDepth {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.EqualFold(filepath.Ext(path), ".lnk") {
				return nil
			}

			resolved, err := ResolvePath(path)
			if err != nil || resolved.Path == "" {
				return nil
			}
			if !looksLaunchable(resolved.Path) {
				return nil
			}

			dedupeKey := strings.ToLower(resolved.Path + "|" + resolved.Args)
			if seen[dedupeKey] {
				return nil
			}
			seen[dedupeKey] = true

			name := strings.TrimSpace(resolved.Name)
			if name == "" {
				name = defaultDisplayName(path)
			}
			out = append(out, StartMenuEntry{
				Name:    name,
				Path:    resolved.Path,
				Args:    resolved.Args,
				WorkDir: resolved.WorkDir,
				LnkPath: path,
				IconSrc: resolved.IconSrc,
				Scope:   root.Scope,
			})
			return nil
		})
	}

	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// looksLaunchable rejects shortcut targets that are not worth adding to a launcher.
func looksLaunchable(path string) bool {
	lower := strings.ToLower(path)
	// Windows ships shortcuts to the web and to uninstallers inside the same folders.
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "ms-") || strings.HasPrefix(lower, "mailto:") {
		return false
	}
	base := strings.ToLower(filepath.Base(lower))
	if strings.HasPrefix(base, "uninstall") || strings.HasPrefix(base, "卸载") {
		return false
	}
	// rundll32 entries in the Start Menu are control-panel applets, not applications.
	if base == "rundll32.exe" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular()
}

func depthOf(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}
