package hook

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Supported mouse trigger identifiers.
const (
	MouseTriggerNone     = "none"
	MouseTriggerMiddle   = "middle"
	MouseTriggerXButton1 = "xbutton1"
	MouseTriggerXButton2 = "xbutton2"
)

const (
	whMouseLL     uintptr = 14
	hcAction      int32   = 0
	wmMButtonDown uintptr = 0x0207
	wmMButtonUp   uintptr = 0x0208
	wmXButtonDown uintptr = 0x020B
	wmXButtonUp   uintptr = 0x020C
	xButton1      uint16  = 0x0001
	xButton2      uint16  = 0x0002
)

const (
	triggerCodeNone int32 = iota
	triggerCodeMiddle
	triggerCodeXButton1
	triggerCodeXButton2
)

type msllHookStruct struct {
	Pt          winPOINT
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

// ValidateMouseTrigger normalizes and validates a mouse trigger string.
func ValidateMouseTrigger(trigger string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(trigger))
	switch normalized {
	case MouseTriggerNone, MouseTriggerMiddle, MouseTriggerXButton1, MouseTriggerXButton2:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid mouse trigger %q: must be one of none, middle, xbutton1, xbutton2", trigger)
	}
}

func triggerToCode(trigger string) int32 {
	switch trigger {
	case MouseTriggerMiddle:
		return triggerCodeMiddle
	case MouseTriggerXButton1:
		return triggerCodeXButton1
	case MouseTriggerXButton2:
		return triggerCodeXButton2
	default:
		return triggerCodeNone
	}
}

// MouseHookManager manages an optional Windows WH_MOUSE_LL low-level mouse hook
// on a dedicated OS-locked thread. When configured to "none", no hook or thread runs.
type MouseHookManager struct {
	onTrigger func()
	triggerCh chan struct{}

	callbackOnce sync.Once
	callback     uintptr

	activeTrigger atomic.Int32
	hhook         atomic.Uintptr
	tid           atomic.Uint32

	swallowedMiddle atomic.Bool
	swallowedX1     atomic.Bool
	swallowedX2     atomic.Bool

	mu         sync.Mutex
	running    bool
	done       chan struct{}
	stopWorker chan struct{}
	workerDone chan struct{}
}

// NewMouseHookManager creates a MouseHookManager that invokes onTrigger asynchronously
// when the configured mouse button is pressed.
func NewMouseHookManager(onTrigger func()) *MouseHookManager {
	return &MouseHookManager{
		onTrigger: onTrigger,
		triggerCh: make(chan struct{}, 1),
	}
}

// Update validates trigger and starts, updates, or stops the low-level mouse hook.
// Setting trigger to "none" unhooks and stops the hook thread completely.
func (m *MouseHookManager) Update(trigger string) error {
	normalized, err := ValidateMouseTrigger(trigger)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	code := triggerToCode(normalized)
	m.activeTrigger.Store(code)

	if code == triggerCodeNone {
		m.stopLocked()
		return nil
	}

	if m.running {
		return nil
	}

	return m.startLocked()
}

// Stop unhooks any active low-level mouse hook and stops the hook thread.
// Stop is idempotent.
func (m *MouseHookManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.activeTrigger.Store(triggerCodeNone)
	m.stopLocked()
}

func (m *MouseHookManager) startLocked() error {
	m.callbackOnce.Do(func() {
		m.callback = syscall.NewCallback(m.lowLevelMouseProc)
	})

	// Clear any residual trigger or swallow state before starting.
	m.resetState()

	stopWorker := make(chan struct{})
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		for {
			select {
			case <-m.triggerCh:
				if m.onTrigger != nil {
					m.onTrigger()
				}
			case <-stopWorker:
				return
			}
		}
	}()

	initDone := make(chan error, 1)
	done := make(chan struct{})

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		var msg winMSG
		procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmNoRemove)
		tid := windows.GetCurrentThreadId()

		hhk, _, err := procSetWindowsHookExW.Call(whMouseLL, m.callback, 0, 0)
		if hhk == 0 {
			if err != nil && err != windows.ERROR_SUCCESS {
				initDone <- fmt.Errorf("SetWindowsHookExW failed: %w", err)
			} else {
				initDone <- errors.New("SetWindowsHookExW failed")
			}
			return
		}

		m.hhook.Store(hhk)
		m.tid.Store(tid)
		initDone <- nil

		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			r := int32(ret)
			if r == 0 || r == -1 || msg.Message == WM_QUIT {
				break
			}
		}

		procUnhookWindowsHookEx.Call(hhk)
		m.hhook.Store(0)
		m.tid.Store(0)
	}()

	if err := <-initDone; err != nil {
		close(stopWorker)
		<-workerDone
		<-done
		return err
	}

	m.running = true
	m.done = done
	m.stopWorker = stopWorker
	m.workerDone = workerDone
	return nil
}

func (m *MouseHookManager) stopLocked() {
	if !m.running {
		return
	}

	if tid := m.tid.Load(); tid != 0 {
		_ = postThreadMessage(tid, WM_QUIT, 0, 0)
	}
	<-m.done

	close(m.stopWorker)
	<-m.workerDone

	m.resetState()
	m.running = false
	m.done = nil
	m.stopWorker = nil
	m.workerDone = nil
}

func (m *MouseHookManager) resetState() {
	m.swallowedMiddle.Store(false)
	m.swallowedX1.Store(false)
	m.swallowedX2.Store(false)
	select {
	case <-m.triggerCh:
	default:
	}
}

func (m *MouseHookManager) lowLevelMouseProc(nCode uintptr, wParam uintptr, lParam unsafe.Pointer) uintptr {
	if int32(nCode) >= hcAction {
		trigger := m.activeTrigger.Load()

		switch wParam {
		case wmMButtonDown:
			if trigger == triggerCodeMiddle {
				m.swallowedMiddle.Store(true)
				m.notifyTrigger()
				return 1
			}
		case wmMButtonUp:
			if m.swallowedMiddle.Swap(false) {
				return 1
			}
		case wmXButtonDown:
			if lParam != nil {
				msll := (*msllHookStruct)(lParam)
				xb := uint16(msll.MouseData >> 16)
				if xb == xButton1 && trigger == triggerCodeXButton1 {
					m.swallowedX1.Store(true)
					m.notifyTrigger()
					return 1
				}
				if xb == xButton2 && trigger == triggerCodeXButton2 {
					m.swallowedX2.Store(true)
					m.notifyTrigger()
					return 1
				}
			}
		case wmXButtonUp:
			if lParam != nil {
				msll := (*msllHookStruct)(lParam)
				xb := uint16(msll.MouseData >> 16)
				if xb == xButton1 && m.swallowedX1.Swap(false) {
					return 1
				}
				if xb == xButton2 && m.swallowedX2.Swap(false) {
					return 1
				}
			}
		}
	}

	r1, _, _ := procCallNextHookEx.Call(m.hhook.Load(), nCode, wParam, uintptr(lParam))
	return r1
}

func (m *MouseHookManager) notifyTrigger() {
	select {
	case m.triggerCh <- struct{}{}:
	default:
	}
}
