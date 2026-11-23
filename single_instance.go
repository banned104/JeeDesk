package main

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	singleInstanceMutexName  = "Local\\JeeDesk-SingleInstance-Mutex-v1"
	singleInstanceClassName  = "JeeDesk-SingleInstance-IPC-Class"
	singleInstanceWindowName = "JeeDesk-SingleInstance-IPC-Window"

	wmJeeDeskShow = 0x8000 + 42 // WM_APP + 42
	hwndMessage   = ^uintptr(2) // HWND_MESSAGE = (HWND)-3

	wmDestroy = 0x0002
	wmClose   = 0x0010

	singleInstanceFindPollInterval = 30 * time.Millisecond
	singleInstanceFindTimeout      = 1500 * time.Millisecond
)

var (
	modKernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procGetModuleHandleW        = modKernel32.NewProc("GetModuleHandleW")
	procFindWindowExW           = modUser32.NewProc("FindWindowExW")
	procAllowSetForegroundWin   = modUser32.NewProc("AllowSetForegroundWindow")
	procPostMessageW            = modUser32.NewProc("PostMessageW")
	procRegisterClassExW        = modUser32.NewProc("RegisterClassExW")
	procUnregisterClassW        = modUser32.NewProc("UnregisterClassW")
	procCreateWindowExW         = modUser32.NewProc("CreateWindowExW")
	procDestroyWindow           = modUser32.NewProc("DestroyWindow")
	procDefWindowProcW          = modUser32.NewProc("DefWindowProcW")
	procGetMessageW             = modUser32.NewProc("GetMessageW")
	procTranslateMessage        = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW        = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage         = modUser32.NewProc("PostQuitMessage")

	singleInstanceMu        sync.RWMutex
	singleInstanceCallbacks = make(map[uintptr]func())
	singleInstanceWndProcCB = windows.NewCallback(singleInstanceWndProc)
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type winMSG struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      winPOINT
}

// SingleInstance holds the primary instance mutex and hidden IPC message window.
type SingleInstance struct {
	mutex windows.Handle
	hwnd  uintptr
	done  chan struct{}

	closeMu sync.Mutex
	closed  bool
}

func singleInstanceWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmJeeDeskShow:
		singleInstanceMu.RLock()
		cb := singleInstanceCallbacks[hwnd]
		singleInstanceMu.RUnlock()
		if cb != nil {
			go cb()
		}
		return 0
	case wmDestroy:
		singleInstanceMu.Lock()
		delete(singleInstanceCallbacks, hwnd)
		singleInstanceMu.Unlock()
		procPostQuitMessage.Call(0)
		return 0
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

// AcquireSingleInstance acquires the named Win32 single-instance mutex.
// If an existing primary instance already holds the mutex, it locates the
// primary's HWND_MESSAGE IPC window, grants foreground permission, posts
// wmJeeDeskShow, and returns (nil, false, nil).
func AcquireSingleInstance(mutexName, className, windowName string, onWake func()) (*SingleInstance, bool, error) {
	if mutexName == "" || className == "" || windowName == "" {
		return nil, false, errors.New("single-instance names must be non-empty")
	}

	mutexPtr, err := windows.UTF16PtrFromString(mutexName)
	if err != nil {
		return nil, false, err
	}
	classPtr, err := windows.UTF16PtrFromString(className)
	if err != nil {
		return nil, false, err
	}
	windowPtr, err := windows.UTF16PtrFromString(windowName)
	if err != nil {
		return nil, false, err
	}

	mutexHandle, err := windows.CreateMutex(nil, false, mutexPtr)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		if mutexHandle != 0 {
			_ = windows.CloseHandle(mutexHandle)
		}
		wakeExistingInstance(classPtr, windowPtr)
		return nil, false, nil
	}
	if err != nil {
		if mutexHandle != 0 {
			_ = windows.CloseHandle(mutexHandle)
		}
		return nil, false, fmt.Errorf("CreateMutex failed: %w", err)
	}

	type initResult struct {
		hwnd uintptr
		err  error
	}
	readyCh := make(chan initResult, 1)
	doneCh := make(chan struct{})

	go func() {
		defer close(doneCh)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hInstance, _, _ := procGetModuleHandleW.Call(0)
		wc := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc:   singleInstanceWndProcCB,
			hInstance:     hInstance,
			lpszClassName: classPtr,
		}
		atom, _, regErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if atom == 0 {
			readyCh <- initResult{err: fmt.Errorf("RegisterClassExW failed: %w", regErr)}
			return
		}
		defer procUnregisterClassW.Call(uintptr(unsafe.Pointer(classPtr)), hInstance)

		hwnd, _, createErr := procCreateWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(classPtr)),
			uintptr(unsafe.Pointer(windowPtr)),
			0,
			0, 0, 0, 0,
			hwndMessage,
			0,
			hInstance,
			0,
		)
		if hwnd == 0 {
			readyCh <- initResult{err: fmt.Errorf("CreateWindowExW failed: %w", createErr)}
			return
		}

		singleInstanceMu.Lock()
		singleInstanceCallbacks[hwnd] = onWake
		singleInstanceMu.Unlock()

		readyCh <- initResult{hwnd: hwnd}

		var msg winMSG
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(ret) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}

		singleInstanceMu.Lock()
		delete(singleInstanceCallbacks, hwnd)
		singleInstanceMu.Unlock()
	}()

	initRes := <-readyCh
	if initRes.err != nil {
		<-doneCh
		_ = windows.CloseHandle(mutexHandle)
		return nil, false, initRes.err
	}

	return &SingleInstance{
		mutex: mutexHandle,
		hwnd:  initRes.hwnd,
		done:  doneCh,
	}, true, nil
}

func wakeExistingInstance(classPtr, windowPtr *uint16) {
	deadline := time.Now().Add(singleInstanceFindTimeout)
	for {
		hwnd, _, _ := procFindWindowExW.Call(
			hwndMessage,
			0,
			uintptr(unsafe.Pointer(classPtr)),
			uintptr(unsafe.Pointer(windowPtr)),
		)
		if hwnd != 0 {
			var pid uint32
			procGetWindowThreadProcID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			if pid != 0 {
				procAllowSetForegroundWin.Call(uintptr(pid))
			}
			procPostMessageW.Call(hwnd, wmJeeDeskShow, 0, 0)
			return
		}
		if time.Now().After(deadline) {
			return
		}
		time.Sleep(singleInstanceFindPollInterval)
	}
}

// Close tears down the IPC message window and releases the single-instance mutex.
// It is safe to call multiple times.
func (s *SingleInstance) Close() {
	if s == nil {
		return
	}
	s.closeMu.Lock()
	if s.closed {
		s.closeMu.Unlock()
		return
	}
	s.closed = true
	s.closeMu.Unlock()

	if s.hwnd != 0 {
		r1, _, _ := procPostMessageW.Call(s.hwnd, wmClose, 0, 0)
		if r1 == 0 {
			procDestroyWindow.Call(s.hwnd)
		}
	}
	if s.done != nil {
		<-s.done
	}
	if s.mutex != 0 {
		_ = windows.CloseHandle(s.mutex)
		s.mutex = 0
	}
}
