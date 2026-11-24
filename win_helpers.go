package main

import (
	"os"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32                 = windows.NewLazySystemDLL("user32.dll")
	modDwmapi                 = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttribute = modDwmapi.NewProc("DwmSetWindowAttribute")
	procEnumWindows           = modUser32.NewProc("EnumWindows")
	procGetClassNameW         = modUser32.NewProc("GetClassNameW")
	procIsWindowVisible       = modUser32.NewProc("IsWindowVisible")
	procIsIconic              = modUser32.NewProc("IsIconic")
	procShowWindow            = modUser32.NewProc("ShowWindow")
	procSetWindowPos          = modUser32.NewProc("SetWindowPos")
	procGetCursorPos          = modUser32.NewProc("GetCursorPos")
	procGetWindowRect         = modUser32.NewProc("GetWindowRect")
	procGetAsyncKeyState      = modUser32.NewProc("GetAsyncKeyState")
	procSetForegroundWindow   = modUser32.NewProc("SetForegroundWindow")
	procBringWindowToTop      = modUser32.NewProc("BringWindowToTop")
	procGetForegroundWindow   = modUser32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcID = modUser32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput     = modUser32.NewProc("AttachThreadInput")
	procMessageBoxW           = modUser32.NewProc("MessageBoxW")
)

const (
	mbOK          = 0x00000000
	mbIconWarning = 0x00000030
	mbTopMost     = 0x00040000

	swShow    = 5
	swRestore = 9

	hwndTopmost   = ^uintptr(0)
	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpShowWindow = 0x0040

	vkLButton = 0x01
	vkRButton = 0x02

	wailsWindowClass = "wailsWindow"
)

type winPOINT struct {
	X int32
	Y int32
}

type winRECT struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

var (
	enumMainMu     sync.Mutex
	enumMainTarget uint32
	enumMainResult uintptr
	enumMainProc   = windows.NewCallback(enumMainWailsWindowProc)
)

func enumMainWailsWindowProc(hwnd, _ uintptr) uintptr {
	var pid uint32
	procGetWindowThreadProcID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != enumMainTarget {
		return 1
	}

	var classBuf [64]uint16
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&classBuf[0])), uintptr(len(classBuf)))
	if n == 0 {
		return 1
	}
	if windows.UTF16ToString(classBuf[:n]) == wailsWindowClass {
		enumMainResult = hwnd
		return 0
	}
	return 1
}

// findMainWailsWindow locates this process's top-level "wailsWindow" HWND via EnumWindows.
func findMainWailsWindow() uintptr {
	enumMainMu.Lock()
	defer enumMainMu.Unlock()

	enumMainTarget = uint32(os.Getpid())
	enumMainResult = 0
	procEnumWindows.Call(enumMainProc, 0)
	return enumMainResult
}

func isWindowVisible(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	r1, _, _ := procIsWindowVisible.Call(hwnd)
	return r1 != 0
}

func isProcessForeground() bool {
	fgHwnd, _, _ := procGetForegroundWindow.Call()
	if fgHwnd == 0 {
		return false
	}
	var fgPID uint32
	procGetWindowThreadProcID.Call(fgHwnd, uintptr(unsafe.Pointer(&fgPID)))
	return fgPID != 0 && fgPID == uint32(os.Getpid())
}

func isCursorInsideWindow(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	var pt winPOINT
	if r1, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); r1 == 0 {
		return false
	}
	var rect winRECT
	if r2, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); r2 == 0 {
		return false
	}
	return pt.X >= rect.Left && pt.X < rect.Right && pt.Y >= rect.Top && pt.Y < rect.Bottom
}

func isMouseButtonDown() bool {
	l, _, _ := procGetAsyncKeyState.Call(vkLButton)
	if uint16(l)&0x8000 != 0 {
		return true
	}
	r, _, _ := procGetAsyncKeyState.Call(vkRButton)
	return uint16(r)&0x8000 != 0
}

// focusWindowHandle reliably brings hwnd to the foreground when triggered by
// a global hotkey, mouse hook, or second-instance launch.
func focusWindowHandle(hwnd uintptr) {
	if hwnd == 0 {
		return
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	iconic, _, _ := procIsIconic.Call(hwnd)
	if iconic != 0 {
		procShowWindow.Call(hwnd, swRestore)
	} else {
		procShowWindow.Call(hwnd, swShow)
	}

	fgHwnd, _, _ := procGetForegroundWindow.Call()
	curTID := windows.GetCurrentThreadId()
	var fgTID uint32
	if fgHwnd != 0 {
		r1, _, _ := procGetWindowThreadProcID.Call(fgHwnd, 0)
		fgTID = uint32(r1)
	}
	var targetTID uint32
	if r1, _, _ := procGetWindowThreadProcID.Call(hwnd, 0); r1 != 0 {
		targetTID = uint32(r1)
	}

	if fgTID != 0 && fgTID != curTID {
		if r1, _, _ := procAttachThreadInput.Call(uintptr(curTID), uintptr(fgTID), 1); r1 != 0 {
			defer procAttachThreadInput.Call(uintptr(curTID), uintptr(fgTID), 0)
		}
	}
	if targetTID != 0 && targetTID != curTID && targetTID != fgTID {
		if r1, _, _ := procAttachThreadInput.Call(uintptr(curTID), uintptr(targetTID), 1); r1 != 0 {
			defer procAttachThreadInput.Call(uintptr(curTID), uintptr(targetTID), 0)
		}
	}

	procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpShowWindow)
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)
}

// DWMWA_WINDOW_CORNER_PREFERENCE = 33 (Windows 11 22H2+); DWMWCP_ROUND = 2.
const (
	dwmwaWindowCornerPreference = 33
	dwmwcpRound                 = 2
)

// roundWindowCorners asks DWM to round the window's corners. Frameless windows
// get square corners by default, which leaves the acrylic backdrop visible
// behind the panel's rounded CSS corners. Errors are ignored: on Windows 10 and
// older Windows 11 builds the attribute is unsupported and the window keeps
// square corners.
func roundWindowCorners(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	preference := int32(dwmwcpRound)
	procDwmSetWindowAttribute.Call(
		hwnd,
		dwmwaWindowCornerPreference,
		uintptr(unsafe.Pointer(&preference)),
		unsafe.Sizeof(preference),
	)
}

// showWarningMessageBox displays a non-crashing top-most warning dialog on Windows.
func showWarningMessageBox(title, message string) {
	titlePtr, err := windows.UTF16PtrFromString(title)
	if err != nil {
		return
	}
	msgPtr, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(msgPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(mbOK|mbIconWarning|mbTopMost),
	)
}
