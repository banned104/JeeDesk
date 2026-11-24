package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modShell32        = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteW = modShell32.NewProc("ShellExecuteW")
)

// Launch starts the given executable, script, document, folder, or URL
// asynchronously using Win32 ShellExecuteW.
func Launch(targetPath, args, workDir string) error {
	file, params, dir, err := resolveLaunchParams(targetPath, args, workDir)
	if err != nil {
		return err
	}
	return shellExecute("open", file, params, dir)
}

// ErrElevationCancelled reports that the user dismissed the UAC prompt.
var ErrElevationCancelled = errors.New("已取消管理员授权")

// LaunchElevated starts the target with the "runas" verb, which makes Windows show a UAC
// prompt. A .lnk target is resolved first so elevation applies to the real program.
func LaunchElevated(targetPath, args, workDir string) error {
	file, params, dir, err := resolveLaunchParams(targetPath, args, workDir)
	if err != nil {
		return err
	}

	verb, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	filePtr, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return fmt.Errorf("invalid target path: %w", err)
	}
	var paramsPtr *uint16
	if params != "" {
		if paramsPtr, err = windows.UTF16PtrFromString(params); err != nil {
			return fmt.Errorf("invalid arguments: %w", err)
		}
	}
	var dirPtr *uint16
	if dir != "" {
		if dirPtr, err = windows.UTF16PtrFromString(dir); err != nil {
			return fmt.Errorf("invalid working directory: %w", err)
		}
	}

	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(filePtr)),
		uintptr(unsafe.Pointer(paramsPtr)),
		uintptr(unsafe.Pointer(dirPtr)),
		uintptr(windows.SW_SHOWNORMAL),
	)
	// ShellExecuteW reports a dismissed UAC prompt as "access denied".
	if ret == 5 {
		return ErrElevationCancelled
	}
	if ret <= 32 {
		return shellExecuteError(ret)
	}
	return nil
}

// OpenFolderAndSelect opens Windows Explorer and selects targetPath.
func OpenFolderAndSelect(targetPath string) error {
	args, err := resolveSelectArgs(targetPath)
	if err != nil {
		return err
	}
	return shellExecute("open", "explorer.exe", args, "")
}

// OpenFile opens filePath with its default associated Windows application.
func OpenFile(filePath string) error {
	cleaned, dir, err := resolveOpenFileParams(filePath)
	if err != nil {
		return err
	}
	return shellExecute("open", cleaned, "", dir)
}

func resolveSelectArgs(targetPath string) (string, error) {
	cleaned := strings.TrimSpace(targetPath)
	if cleaned == "" {
		return "", errors.New("target path cannot be empty")
	}
	cleaned = expandEnvVars(cleaned)
	cleaned = filepath.Clean(cleaned)
	return `/select,"` + cleaned + `"`, nil
}

func resolveOpenFileParams(filePath string) (cleaned, dir string, err error) {
	cleaned = strings.TrimSpace(filePath)
	if cleaned == "" {
		return "", "", errors.New("file path cannot be empty")
	}
	cleaned = expandEnvVars(cleaned)
	cleaned = filepath.Clean(cleaned)
	return cleaned, filepath.Dir(cleaned), nil
}

func resolveLaunchParams(targetPath, args, workDir string) (file, params, dir string, err error) {
	target := strings.TrimSpace(targetPath)
	if target == "" {
		return "", "", "", errors.New("target path cannot be empty")
	}

	target = expandEnvVars(target)
	uri := isURI(target)
	if !uri {
		target = filepath.Clean(target)
	}

	params = strings.TrimSpace(args)
	dir = strings.TrimSpace(workDir)
	if dir != "" {
		dir = filepath.Clean(expandEnvVars(dir))
	} else if !uri {
		if info, statErr := os.Stat(target); statErr == nil {
			if info.IsDir() {
				dir = target
			} else {
				dir = filepath.Dir(target)
			}
		}
	}

	// Windows default "open" verb for .ps1 often opens Notepad instead of running
	// the script, so invoke PowerShell explicitly via ShellExecuteW.
	if !uri && strings.EqualFold(filepath.Ext(target), ".ps1") {
		psArgs := fmt.Sprintf(`-NoProfile -ExecutionPolicy Bypass -File "%s"`, target)
		if params != "" {
			psArgs += " " + params
		}
		return findPowerShell(), psArgs, dir, nil
	}

	return target, params, dir, nil
}

func shellExecute(verb, file, params, dir string) error {
	verbPtr, err := windows.UTF16PtrFromString(verb)
	if err != nil {
		return fmt.Errorf("invalid verb: %w", err)
	}
	filePtr, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return fmt.Errorf("invalid target path: %w", err)
	}

	var paramsPtr *uint16
	if params != "" {
		paramsPtr, err = windows.UTF16PtrFromString(params)
		if err != nil {
			return fmt.Errorf("invalid arguments: %w", err)
		}
	}

	var dirPtr *uint16
	if dir != "" {
		dirPtr, err = windows.UTF16PtrFromString(dir)
		if err != nil {
			return fmt.Errorf("invalid working directory: %w", err)
		}
	}

	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(filePtr)),
		uintptr(unsafe.Pointer(paramsPtr)),
		uintptr(unsafe.Pointer(dirPtr)),
		uintptr(windows.SW_SHOWNORMAL),
	)
	if ret <= 32 {
		return shellExecuteError(ret)
	}
	return nil
}

func shellExecuteError(code uintptr) error {
	if code > 32 {
		return nil
	}
	switch code {
	case 0:
		return errors.New("ShellExecuteW failed: operating system is out of memory or resources (code 0)")
	case 2:
		return errors.New("ShellExecuteW failed: file not found (code 2)")
	case 3:
		return errors.New("ShellExecuteW failed: path not found (code 3)")
	case 5:
		return errors.New("ShellExecuteW failed: access denied (code 5)")
	case 8:
		return errors.New("ShellExecuteW failed: insufficient memory (code 8)")
	case 11:
		return errors.New("ShellExecuteW failed: invalid executable format (code 11)")
	case 26:
		return errors.New("ShellExecuteW failed: sharing violation (code 26)")
	case 27:
		return errors.New("ShellExecuteW failed: file name association is incomplete or invalid (code 27)")
	case 28:
		return errors.New("ShellExecuteW failed: DDE transaction timed out (code 28)")
	case 29:
		return errors.New("ShellExecuteW failed: DDE transaction failed (code 29)")
	case 30:
		return errors.New("ShellExecuteW failed: DDE transaction busy (code 30)")
	case 31:
		return errors.New("ShellExecuteW failed: no application associated with this file type (code 31)")
	case 32:
		return errors.New("ShellExecuteW failed: specified DLL not found (code 32)")
	default:
		return fmt.Errorf("ShellExecuteW failed with error code %d", code)
	}
}

func expandEnvVars(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	src, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return s
	}

	buf := make([]uint16, 260)
	n, err := windows.ExpandEnvironmentStrings(src, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		return s
	}
	if n > uint32(len(buf)) {
		buf = make([]uint16, n)
		n, err = windows.ExpandEnvironmentStrings(src, &buf[0], uint32(len(buf)))
		if err != nil || n == 0 || n > uint32(len(buf)) {
			return s
		}
	}
	return windows.UTF16ToString(buf[:n])
}

// isURI reports whether s is a protocol URL rather than a file path. Windows has thousands of
// registered schemes (obsidian:, vscode:, steam:, ms-settings: …), so the check accepts any
// well-formed scheme instead of a list that would keep breaking installed applications. A drive
// letter is excluded by VolumeName and by requiring at least two scheme characters.
func isURI(s string) bool {
	if filepath.VolumeName(s) != "" {
		return false
	}
	colonIdx := strings.IndexByte(s, ':')
	if colonIdx < 2 {
		return false
	}
	scheme := s[:colonIdx]
	for i := 0; i < len(scheme); i++ {
		c := scheme[i]
		isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isDigit := c >= '0' && c <= '9'
		if i == 0 && !isAlpha {
			return false
		}
		if !isAlpha && !isDigit && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	return true
}

func findPowerShell() string {
	if existsInPath("powershell.exe") {
		return "powershell.exe"
	}
	if sysRoot := os.Getenv("SystemRoot"); sysRoot != "" {
		winPS := filepath.Join(sysRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		if info, err := os.Stat(winPS); err == nil && !info.IsDir() {
			return winPS
		}
	}
	if existsInPath("pwsh.exe") {
		return "pwsh.exe"
	}
	return "powershell.exe"
}

func existsInPath(binary string) bool {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, binary)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}
