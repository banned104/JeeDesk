package resolver

import (
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/parsiya/golnk"
)

func TestIconHash(t *testing.T) {
	h1 := IconHash(`C:\Windows\System32\notepad.exe`)
	h2 := IconHash(`c:\windows\system32\NOTEPAD.EXE`)
	h3 := IconHash(`c:/Windows/System32/./notepad.exe`)

	if len(h1) != 32 {
		t.Fatalf("expected 32-char MD5 hex string, got %q (len %d)", h1, len(h1))
	}
	if h1 != h2 || h1 != h3 {
		t.Fatalf("expected identical hashes across case/normalization, got %q, %q, %q", h1, h2, h3)
	}

	other := IconHash(`C:\Windows\System32\calc.exe`)
	if other == h1 {
		t.Fatalf("expected different paths to produce different hashes")
	}
}

func TestResolvePathExecutableAndDirectory(t *testing.T) {
	notepad := `C:\Windows\System32\notepad.exe`
	if _, err := os.Stat(notepad); err != nil {
		t.Skipf("notepad.exe not present: %v", err)
	}

	got, err := ResolvePath(notepad)
	if err != nil {
		t.Fatalf("ResolvePath(%q) failed: %v", notepad, err)
	}
	if got.Name != "notepad" {
		t.Errorf("got Name %q, want %q", got.Name, "notepad")
	}
	if !strings.EqualFold(got.Path, notepad) {
		t.Errorf("got Path %q, want %q", got.Path, notepad)
	}
	if !strings.EqualFold(got.IconSrc, notepad) {
		t.Errorf("got IconSrc %q, want %q", got.IconSrc, notepad)
	}

	// Directory with a dot in its name should keep full folder name.
	parentDir := t.TempDir()
	dottedDir := filepath.Join(parentDir, "my.project.v1")
	if err := os.MkdirAll(dottedDir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	dirTarget, err := ResolvePath(dottedDir + string(filepath.Separator))
	if err != nil {
		t.Fatalf("ResolvePath(dir) failed: %v", err)
	}
	if dirTarget.Name != "my.project.v1" {
		t.Errorf("got dir Name %q, want %q", dirTarget.Name, "my.project.v1")
	}
	if dirTarget.Path != dottedDir {
		t.Errorf("got dir Path %q, want %q", dirTarget.Path, dottedDir)
	}
	if dirTarget.IconSrc != dottedDir {
		t.Errorf("got dir IconSrc %q, want %q", dirTarget.IconSrc, dottedDir)
	}

	if _, err := ResolvePath("   "); err == nil {
		t.Errorf("expected error for empty input path")
	}
}

func TestExtractAndCacheIcon(t *testing.T) {
	iconsDir := filepath.Join(t.TempDir(), "icons")

	notepad := `C:\Windows\System32\notepad.exe`
	if _, err := os.Stat(notepad); err != nil {
		t.Skipf("notepad.exe not present: %v", err)
	}

	relExe, err := ExtractAndCacheIcon(notepad, iconsDir)
	if err != nil {
		t.Fatalf("ExtractAndCacheIcon(notepad) failed: %v", err)
	}
	wantRelExe := "icons/" + IconHash(notepad) + ".png"
	if relExe != wantRelExe {
		t.Errorf("got rel path %q, want %q", relExe, wantRelExe)
	}
	assertValidNonTransparentPNG(t, filepath.Join(iconsDir, IconHash(notepad)+".png"))

	// Folder icon via SHGetFileInfoW fallback.
	folder := t.TempDir()
	relDir, err := ExtractAndCacheIcon(folder, iconsDir)
	if err != nil {
		t.Fatalf("ExtractAndCacheIcon(folder) failed: %v", err)
	}
	wantRelDir := "icons/" + IconHash(folder) + ".png"
	if relDir != wantRelDir {
		t.Errorf("got rel path %q, want %q", relDir, wantRelDir)
	}
	assertValidNonTransparentPNG(t, filepath.Join(iconsDir, IconHash(folder)+".png"))

	// Non-executable script/text file via SHGetFileInfoW fallback.
	batFile := filepath.Join(t.TempDir(), "run.bat")
	if err := os.WriteFile(batFile, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatalf("write bat file: %v", err)
	}
	relBat, err := ExtractAndCacheIcon(batFile, iconsDir)
	if err != nil {
		t.Fatalf("ExtractAndCacheIcon(bat) failed: %v", err)
	}
	if relBat != "icons/"+IconHash(batFile)+".png" {
		t.Errorf("unexpected rel path for bat: %q", relBat)
	}
	assertValidNonTransparentPNG(t, filepath.Join(iconsDir, IconHash(batFile)+".png"))
}

func TestResolvePathLnkFallbackAndRealShortcut(t *testing.T) {
	tmpDir := t.TempDir()

	// Invalid .lnk should gracefully fall back without returning an error.
	badLnk := filepath.Join(tmpDir, "Broken Shortcut.lnk")
	if err := os.WriteFile(badLnk, []byte("not a valid lnk"), 0o644); err != nil {
		t.Fatalf("write bad lnk: %v", err)
	}
	fallback, err := ResolvePath(badLnk)
	if err != nil {
		t.Fatalf("expected graceful fallback on invalid .lnk, got error: %v", err)
	}
	if fallback.Name != "Broken Shortcut" {
		t.Errorf("got Name %q, want %q", fallback.Name, "Broken Shortcut")
	}
	if fallback.Path != badLnk {
		t.Errorf("got Path %q, want %q", fallback.Path, badLnk)
	}
	if fallback.IconSrc != badLnk {
		t.Errorf("got IconSrc %q, want %q", fallback.IconSrc, badLnk)
	}

	// Create a real .lnk shortcut via PowerShell WScript.Shell.
	notepad := `C:\Windows\System32\notepad.exe`
	if _, err := os.Stat(notepad); err != nil {
		t.Skipf("notepad.exe not present: %v", err)
	}

	realLnk := filepath.Join(tmpDir, "Launch Notepad.lnk")
	psScript := `$s = (New-Object -ComObject WScript.Shell).CreateShortcut('` +
		strings.ReplaceAll(realLnk, `'`, `''`) + `'); ` +
		`$s.TargetPath = '%SystemRoot%\System32\notepad.exe'; ` +
		`$s.Arguments = '/A sample.txt'; ` +
		`$s.WorkingDirectory = '%SystemRoot%\System32'; ` +
		`$s.IconLocation = '%SystemRoot%\System32\notepad.exe,0'; ` +
		`$s.Save()`

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psScript)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("unable to create real .lnk via WScript.Shell: %v (%s)", err, string(out))
	}

	resolved, err := ResolvePath(realLnk)
	if err != nil {
		t.Fatalf("ResolvePath(real .lnk) failed: %v", err)
	}
	if resolved.Name != "Launch Notepad" {
		t.Errorf("got Name %q, want %q", resolved.Name, "Launch Notepad")
	}
	if !strings.EqualFold(resolved.Path, notepad) {
		t.Errorf("got Path %q, want %q", resolved.Path, notepad)
	}
	if resolved.Args != "/A sample.txt" {
		t.Errorf("got Args %q, want %q", resolved.Args, "/A sample.txt")
	}
	if !strings.EqualFold(resolved.WorkDir, `C:\Windows\System32`) {
		t.Errorf("got WorkDir %q, want %q", resolved.WorkDir, `C:\Windows\System32`)
	}
	if !strings.EqualFold(resolved.IconSrc, notepad) {
		t.Errorf("got IconSrc %q, want %q", resolved.IconSrc, notepad)
	}
}

func TestResolveLnkPriorityAndEnvBlock(t *testing.T) {
	tmpDir := t.TempDir()
	relTarget := filepath.Join(tmpDir, "tool.exe")
	if err := os.WriteFile(relTarget, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write stub exe: %v", err)
	}
	lnkPath := filepath.Join(tmpDir, "sub", "tool.lnk")

	// Priority 1: LinkInfo LocalBasePathUnicode + CommonPathSuffixUnicode.
	notepad := `C:\Windows\System32\notepad.exe`
	if _, err := os.Stat(notepad); err == nil {
		var f1 lnk.LnkFile
		f1.LinkInfo.LocalBasePathUnicode = `C:\Windows\System32`
		f1.LinkInfo.CommonPathSuffixUnicode = `notepad.exe`
		f1.StringData.RelativePath = filepath.Join("..", "tool.exe")
		got := resolveLnkTargetPath(lnkPath, f1)
		if !strings.EqualFold(got, notepad) {
			t.Errorf("priority 1 got %q, want %q", got, notepad)
		}
	}

	// Priority 2: StringData.RelativePath when LinkInfo path does not exist.
	var f2 lnk.LnkFile
	f2.LinkInfo.LocalBasePath = `Z:\nonexistent\missing.exe`
	f2.StringData.RelativePath = filepath.Join("..", "tool.exe")
	if got := resolveLnkTargetPath(lnkPath, f2); !strings.EqualFold(got, relTarget) {
		t.Errorf("priority 2 got %q, want %q", got, relTarget)
	}

	// Priority 3: EnvironmentVariableDataBlock UTF-16LE target.
	var f3 lnk.LnkFile
	blockData := make([]byte, ansiTargetFieldLen+unicodeTargetFieldLen)
	copy(blockData[:ansiTargetFieldLen], `%SystemRoot%\System32\cmd.exe`)
	copy(blockData[ansiTargetFieldLen:], utf16EncodeForTest(`%SystemRoot%\System32\cmd.exe`))
	f3.DataBlocks.Blocks = append(f3.DataBlocks.Blocks, lnk.ExtraDataBlock{
		Size:      uint32(8 + len(blockData)),
		Signature: envVarDataBlockSig,
		Data:      blockData,
	})
	if got := resolveLnkTargetPath(lnkPath, f3); !strings.EqualFold(got, `C:\Windows\System32\cmd.exe`) {
		t.Errorf("priority 3 got %q, want %q", got, `C:\Windows\System32\cmd.exe`)
	}
}

func utf16EncodeForTest(s string) []byte {
	u16 := utf16.Encode([]rune(s))
	out := make([]byte, len(u16)*2)
	for i, v := range u16 {
		out[i*2] = byte(v)
		out[i*2+1] = byte(v >> 8)
	}
	return out
}

func assertValidNonTransparentPNG(t *testing.T, pngPath string) {
	t.Helper()

	f, err := os.Open(pngPath)
	if err != nil {
		t.Fatalf("open png %q: %v", pngPath, err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode png %q: %v", pngPath, err)
	}

	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		t.Fatalf("invalid PNG dimensions for %q: %v", pngPath, bounds)
	}

	if !hasOpaquePixel(img) {
		t.Fatalf("PNG %q has no non-transparent pixels", pngPath)
	}
}

func hasOpaquePixel(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				return true
			}
		}
	}
	return false
}
