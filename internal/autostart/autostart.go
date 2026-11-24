package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName  = "JeeDesk"
)

// Command quotes an executable path and appends nothing else, so Windows starts the
// application itself rather than a launcher that guessed the wrong target.
func Command(exePath string) string {
	cleaned := strings.TrimSpace(exePath)
	if cleaned == "" {
		return ""
	}
	cleaned = filepath.Clean(cleaned)
	if strings.HasPrefix(cleaned, `"`) && strings.HasSuffix(cleaned, `"`) {
		return cleaned
	}
	return `"` + cleaned + `"`
}

// CurrentExe returns the executable path that autostart would register.
func CurrentExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("定位可执行文件失败: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return filepath.Clean(exe), nil
	}
	return filepath.Clean(resolved), nil
}

// Enabled reports whether the Run entry exists and points at the current executable.
func Enabled() (bool, error) {
	exe, err := CurrentExe()
	if err != nil {
		return false, err
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false, nil
	}
	defer key.Close()

	value, _, err := key.GetStringValue(valueName)
	if err != nil {
		return false, nil
	}
	return sameCommand(value, Command(exe)), nil
}

// Set registers or removes the Run entry for the current executable.
func Set(enabled bool) error {
	exe, err := CurrentExe()
	if err != nil {
		return err
	}
	return SetCommand(enabled, exe)
}

// SetCommand registers or removes the Run entry with an explicit executable path.
func SetCommand(enabled bool, exePath string) error {
	command := Command(exePath)
	if enabled && command == "" {
		return errors.New("开机启动路径为空")
	}

	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("打开开机启动注册表项失败: %w", err)
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("关闭开机启动失败: %w", err)
		}
		return nil
	}
	if err := key.SetStringValue(valueName, command); err != nil {
		return fmt.Errorf("写入开机启动失败: %w", err)
	}
	return nil
}

func sameCommand(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
