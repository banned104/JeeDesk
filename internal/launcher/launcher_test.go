package launcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveLaunchParams_EmptyTarget(t *testing.T) {
	for _, target := range []string{"", "   ", "\t\n"} {
		if _, _, _, err := resolveLaunchParams(target, "", ""); err == nil {
			t.Fatalf("expected error for empty target %q, got nil", target)
		}
	}
}

func TestResolveLaunchParams_ExeEmptyWorkDir(t *testing.T) {
	tmpDir := t.TempDir()
	exePath := filepath.Join(tmpDir, "app.exe")
	if err := os.WriteFile(exePath, []byte("stub"), 0o644); err != nil {
		t.Fatalf("write stub exe: %v", err)
	}

	file, params, dir, err := resolveLaunchParams("  "+exePath+"  ", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file != exePath {
		t.Fatalf("file = %q, want %q", file, exePath)
	}
	if params != "" {
		t.Fatalf("params = %q, want empty", params)
	}
	if dir != tmpDir {
		t.Fatalf("dir = %q, want %q", dir, tmpDir)
	}
}

func TestResolveLaunchParams_ExeCustomWorkDirAndArgs(t *testing.T) {
	tmpDir := t.TempDir()
	customDir := t.TempDir()
	exePath := filepath.Join(tmpDir, "tool.exe")

	file, params, dir, err := resolveLaunchParams(exePath, "  --flag value  ", "  "+customDir+`\  `)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file != exePath {
		t.Fatalf("file = %q, want %q", file, exePath)
	}
	if params != "--flag value" {
		t.Fatalf("params = %q, want %q", params, "--flag value")
	}
	if dir != customDir {
		t.Fatalf("dir = %q, want %q", dir, customDir)
	}
}

func TestResolveLaunchParams_DirectoryTarget(t *testing.T) {
	tmpDir := t.TempDir()

	file, params, dir, err := resolveLaunchParams(tmpDir+`\`, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file != tmpDir {
		t.Fatalf("file = %q, want %q", file, tmpDir)
	}
	if params != "" {
		t.Fatalf("params = %q, want empty", params)
	}
	if dir != tmpDir {
		t.Fatalf("dir = %q, want %q", dir, tmpDir)
	}
}

func TestResolveLaunchParams_BatFile(t *testing.T) {
	tmpDir := t.TempDir()
	batPath := filepath.Join(tmpDir, "build script.bat")
	if err := os.WriteFile(batPath, []byte("@echo off\r\n"), 0o644); err != nil {
		t.Fatalf("write bat: %v", err)
	}

	file, params, dir, err := resolveLaunchParams(batPath, "/c release", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file != batPath {
		t.Fatalf("file = %q, want %q", file, batPath)
	}
	if params != "/c release" {
		t.Fatalf("params = %q, want %q", params, "/c release")
	}
	if dir != tmpDir {
		t.Fatalf("dir = %q, want %q", dir, tmpDir)
	}
}

func TestResolveLaunchParams_PS1QuotingWithSpaces(t *testing.T) {
	baseDir := t.TempDir()
	scriptDir := filepath.Join(baseDir, "my scripts")
	if err := os.MkdirAll(scriptDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ps1Path := filepath.Join(scriptDir, "hello world.PS1")
	if err := os.WriteFile(ps1Path, []byte("Write-Output 'hi'\r\n"), 0o644); err != nil {
		t.Fatalf("write ps1: %v", err)
	}

	file, params, dir, err := resolveLaunchParams(ps1Path, "-Name \"JeeDesk\"", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file != "powershell.exe" && file != "pwsh.exe" {
		t.Fatalf("file = %q, want powershell.exe or pwsh.exe", file)
	}
	wantParams := `-NoProfile -ExecutionPolicy Bypass -File "` + ps1Path + `" -Name "JeeDesk"`
	if params != wantParams {
		t.Fatalf("params = %q, want %q", params, wantParams)
	}
	if dir != scriptDir {
		t.Fatalf("dir = %q, want %q", dir, scriptDir)
	}
}

func TestResolveLaunchParams_URLAndEnvVars(t *testing.T) {
	urlTarget := "https://example.com/docs?q=1&b=2"
	file, params, dir, err := resolveLaunchParams(urlTarget, "", "")
	if err != nil {
		t.Fatalf("unexpected error for URL: %v", err)
	}
	if file != urlTarget {
		t.Fatalf("URL mangled: got %q, want %q", file, urlTarget)
	}
	if params != "" || dir != "" {
		t.Fatalf("unexpected params/dir for URL: params=%q dir=%q", params, dir)
	}

	t.Setenv("JEEDESK_TEST_ROOT", `C:\CustomRoot`)
	file, _, dir, err = resolveLaunchParams(`%JEEDESK_TEST_ROOT%\bin\app.exe`, "", `%JEEDESK_TEST_ROOT%\work`)
	if err != nil {
		t.Fatalf("unexpected error for env vars: %v", err)
	}
	if file != `C:\CustomRoot\bin\app.exe` {
		t.Fatalf("expanded file = %q, want %q", file, `C:\CustomRoot\bin\app.exe`)
	}
	if dir != `C:\CustomRoot\work` {
		t.Fatalf("expanded dir = %q, want %q", dir, `C:\CustomRoot\work`)
	}
}

func TestShellExecuteError(t *testing.T) {
	cases := []struct {
		code uintptr
		want string
	}{
		{0, "out of memory"},
		{2, "file not found"},
		{3, "path not found"},
		{5, "access denied"},
		{8, "insufficient memory"},
		{11, "invalid executable format"},
		{26, "sharing violation"},
		{27, "association is incomplete"},
		{28, "timed out"},
		{29, "DDE transaction failed"},
		{30, "DDE transaction busy"},
		{31, "no application associated"},
		{32, "DLL not found"},
		{15, "error code 15"},
	}

	for _, tc := range cases {
		err := shellExecuteError(tc.code)
		if err == nil {
			t.Fatalf("code %d: expected non-nil error", tc.code)
		}
		if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
			t.Fatalf("code %d: error %q does not contain %q", tc.code, err.Error(), tc.want)
		}
	}

	if err := shellExecuteError(33); err != nil {
		t.Fatalf("code 33: expected nil error, got %v", err)
	}
	if err := shellExecuteError(42); err != nil {
		t.Fatalf("code 42: expected nil error, got %v", err)
	}
}

func TestOpenFolderAndSelectAndOpenFileValidation(t *testing.T) {
	if err := Launch("", "", ""); err == nil {
		t.Fatal("Launch(\"\") expected error, got nil")
	}
	if err := OpenFolderAndSelect(""); err == nil {
		t.Fatal("OpenFolderAndSelect(\"\") expected error, got nil")
	}
	if err := OpenFolderAndSelect("   "); err == nil {
		t.Fatal("OpenFolderAndSelect(\"   \") expected error, got nil")
	}
	if err := OpenFile(""); err == nil {
		t.Fatal("OpenFile(\"\") expected error, got nil")
	}
	if err := OpenFile("   "); err == nil {
		t.Fatal("OpenFile(\"   \") expected error, got nil")
	}

	selectArgs, err := resolveSelectArgs(`  C:\Users\Test\foo\..\config.json  `)
	if err != nil {
		t.Fatalf("resolveSelectArgs unexpected error: %v", err)
	}
	wantSelect := `/select,"C:\Users\Test\config.json"`
	if selectArgs != wantSelect {
		t.Fatalf("selectArgs = %q, want %q", selectArgs, wantSelect)
	}

	cleaned, dir, err := resolveOpenFileParams(`  C:\Users\Test\foo\..\config.json  `)
	if err != nil {
		t.Fatalf("resolveOpenFileParams unexpected error: %v", err)
	}
	if cleaned != `C:\Users\Test\config.json` {
		t.Fatalf("cleaned = %q, want %q", cleaned, `C:\Users\Test\config.json`)
	}
	if dir != `C:\Users\Test` {
		t.Fatalf("dir = %q, want %q", dir, `C:\Users\Test`)
	}
}
