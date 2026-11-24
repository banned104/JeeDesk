package resolver

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/parsiya/golnk"
	"golang.org/x/sys/windows"
)

const (
	// envVarDataBlockSig is the MS-SHLLINK EnvironmentVariableDataBlock signature.
	envVarDataBlockSig uint32 = 0xA0000001
	// iconEnvDataBlockSig is the MS-SHLLINK IconEnvironmentDataBlock signature.
	iconEnvDataBlockSig uint32 = 0xA0000007

	ansiTargetFieldLen    = 260
	unicodeTargetFieldLen = 520
)

// ResolvedTarget holds the resolved metadata for a dropped or selected file,
// folder, or Windows shortcut (.lnk).
type ResolvedTarget struct {
	Name    string // clean display name (file basename without extension, or folder name)
	Path    string // real target path (resolved if .lnk)
	Args    string // command-line arguments from .lnk
	WorkDir string // working directory from .lnk
	IconSrc string // icon source path (from .lnk IconLocation if valid file, else Path)
}

// ResolvePath resolves a file, directory, or Windows .lnk shortcut into a
// normalized ResolvedTarget. If .lnk parsing fails or yields no target path,
// it gracefully falls back to the .lnk file path itself so the caller can
// still register the shortcut.
func ResolvePath(inputPath string) (ResolvedTarget, error) {
	trimmed := strings.TrimSpace(inputPath)
	if trimmed == "" {
		return ResolvedTarget{}, fmt.Errorf("input path is empty")
	}

	cleaned := filepath.Clean(trimmed)
	name := defaultDisplayName(cleaned)

	if !strings.EqualFold(filepath.Ext(cleaned), ".lnk") {
		return ResolvedTarget{
			Name:    name,
			Path:    cleaned,
			IconSrc: cleaned,
		}, nil
	}

	fallback := ResolvedTarget{
		Name:    name,
		Path:    cleaned,
		IconSrc: cleaned,
	}

	lnkFile, err := lnk.File(cleaned)
	if err != nil {
		return fallback, nil
	}

	targetPath := resolveLnkTargetPath(cleaned, lnkFile)
	if targetPath == "" {
		return fallback, nil
	}
	targetPath = filepath.Clean(targetPath)

	args := strings.TrimSpace(lnkFile.StringData.CommandLineArguments)
	workDir := cleanExpandedPath(lnkFile.StringData.WorkingDir)

	iconSrc := resolveLnkIconSource(lnkFile, targetPath)

	return ResolvedTarget{
		Name:    name,
		Path:    targetPath,
		Args:    args,
		WorkDir: workDir,
		IconSrc: iconSrc,
	}, nil
}

// DisplayName derives the item name JeeDesk shows for a path: the file name without the
// extension, a folder name as is, and a shortcut dropped ".lnk". AddItem and the drop handler share
// it so a manually added target and a dropped one get the same name.
func DisplayName(path string) string {
	cleaned := strings.TrimRight(strings.TrimSpace(path), `\/`)
	if cleaned == "" {
		return strings.TrimSpace(path)
	}
	return defaultDisplayName(cleaned)
}

func defaultDisplayName(cleaned string) string {
	base := filepath.Base(cleaned)
	ext := filepath.Ext(base)

	if strings.EqualFold(ext, ".lnk") {
		if trimmed := base[:len(base)-len(ext)]; trimmed != "" {
			return trimmed
		}
		return base
	}

	if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
		return base
	}

	if trimmed := strings.TrimSuffix(base, ext); trimmed != "" {
		return trimmed
	}
	return base
}

func resolveLnkTargetPath(lnkPath string, lnkFile lnk.LnkFile) string {
	// 1. LinkInfo LocalBasePath(Unicode) + CommonPathSuffix(Unicode).
	candLocal := cleanExpandedPath(combineLinkInfoPath(lnkFile.LinkInfo))
	if candLocal != "" && pathExists(candLocal) {
		return candLocal
	}

	// 2. StringData.RelativePath resolved against the .lnk parent directory.
	if rel := expandEnvWindows(strings.TrimSpace(lnkFile.StringData.RelativePath)); rel != "" {
		candRel := filepath.Clean(filepath.Join(filepath.Dir(lnkPath), rel))
		if pathExists(candRel) {
			return candRel
		}
	}

	// 3. ExtraData EnvironmentVariableDataBlock target.
	if envTarget := extractEnvBlockTarget(lnkFile.DataBlocks, envVarDataBlockSig); envTarget != "" {
		if expanded := cleanExpandedPath(envTarget); expanded != "" {
			return expanded
		}
	}

	return candLocal
}

func combineLinkInfoPath(info lnk.LinkInfoSection) string {
	base := strings.TrimSpace(info.LocalBasePathUnicode)
	suffix := strings.TrimSpace(info.CommonPathSuffixUnicode)
	if base == "" {
		base = strings.TrimSpace(info.LocalBasePath)
	}
	if suffix == "" {
		suffix = strings.TrimSpace(info.CommonPathSuffix)
	}
	if base == "" {
		return ""
	}
	if suffix == "" {
		return base
	}
	return filepath.Join(base, strings.TrimLeft(suffix, `\/`))
}

func resolveLnkIconSource(lnkFile lnk.LnkFile, fallbackTarget string) string {
	iconRaw := strings.TrimSpace(lnkFile.StringData.IconLocation)
	if iconRaw == "" {
		iconRaw = extractEnvBlockTarget(lnkFile.DataBlocks, iconEnvDataBlockSig)
	}
	if iconRaw == "" {
		return fallbackTarget
	}

	expanded := expandEnvWindows(iconRaw)
	if expanded == "" {
		return fallbackTarget
	}

	candidate := stripIconIndexSuffix(expanded)
	cleaned := filepath.Clean(candidate)
	if fileExists(cleaned) {
		return cleaned
	}
	return fallbackTarget
}

func stripIconIndexSuffix(path string) string {
	if fileExists(path) {
		return path
	}
	idx := strings.LastIndex(path, ",")
	if idx <= 0 {
		return path
	}
	tail := strings.TrimSpace(path[idx+1:])
	if tail == "" {
		return path
	}
	for i, r := range tail {
		if i == 0 && (r == '-' || r == '+') {
			continue
		}
		if r < '0' || r > '9' {
			return path
		}
	}
	return strings.TrimSpace(path[:idx])
}

func extractEnvBlockTarget(extra lnk.ExtraDataSection, sig uint32) string {
	for _, block := range extra.Blocks {
		if block.Signature != sig {
			continue
		}
		if len(block.Data) >= ansiTargetFieldLen+unicodeTargetFieldLen {
			if s := decodeUTF16Z(block.Data[ansiTargetFieldLen : ansiTargetFieldLen+unicodeTargetFieldLen]); s != "" {
				return s
			}
		}
		if len(block.Data) >= ansiTargetFieldLen {
			if s := decodeANSIZ(block.Data[:ansiTargetFieldLen]); s != "" {
				return s
			}
		}
	}
	return ""
}

func decodeUTF16Z(b []byte) string {
	n := len(b) / 2
	u16 := make([]uint16, 0, n)
	for i := 0; i+1 < len(b); i += 2 {
		v := uint16(b[i]) | uint16(b[i+1])<<8
		if v == 0 {
			break
		}
		u16 = append(u16, v)
	}
	if len(u16) == 0 {
		return ""
	}
	return strings.TrimSpace(string(utf16.Decode(u16)))
}

func decodeANSIZ(b []byte) string {
	if idx := bytes.IndexByte(b, 0); idx >= 0 {
		b = b[:idx]
	}
	return strings.TrimSpace(string(b))
}

func cleanExpandedPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	expanded := expandEnvWindows(trimmed)
	if expanded == "" {
		return ""
	}
	return filepath.Clean(expanded)
}

func expandEnvWindows(s string) string {
	if s == "" || !strings.Contains(s, "%") {
		return s
	}
	src, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return s
	}
	n, err := windows.ExpandEnvironmentStrings(src, nil, 0)
	if err != nil || n == 0 {
		return s
	}
	buf := make([]uint16, n)
	if _, err := windows.ExpandEnvironmentStrings(src, &buf[0], n); err != nil {
		return s
	}
	return windows.UTF16ToString(buf)
}

func pathExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func fileExists(p string) bool {
	if p == "" {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
