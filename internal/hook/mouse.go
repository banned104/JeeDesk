package hook

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
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

const (
	// mouseHookEventTimeout is how long the hook may stay silent while the
	// system keeps receiving input before it is considered dead.
	mouseHookEventTimeout = 6 * time.Second
	mouseHookWatchdogTick = 3 * time.Second
	mouseHookStopTimeout  = 2 * time.Second
)

type lastInputInfo struct {
	CbSize uint32
	Time   uint32
}

var (
	modKernel32Hook         = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount64      = modKernel32Hook.NewProc("GetTickCount64")
	procGetLastInputInfoRef = user32.NewProc("GetLastInputInfo")
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
	swallowInput  atomic.Bool
	lastEventTick atomic.Uint64
	rehookCount   atomic.Uint32

	swallowedMiddle atomic.Bool
	swallowedX1     atomic.Bool
	swallowedX2     atomic.Bool

	mu           sync.Mutex
	running      bool
	done         chan struct{}
	stopWorker   chan struct{}
	workerDone   chan struct{}
	watchdogStop chan struct{}
	watchdogDone chan struct{}
}

// NewMouseHookManager creates a MouseHookManager that invokes onTrigger asynchronously
// when the configured mouse button is pressed.
func NewMouseHookManager(onTrigger func()) *MouseHookManager {
	m := &MouseHookManager{
		onTrigger: onTrigger,
		triggerCh: make(chan struct{}, 1),
	}
	m.swallowInput.Store(true)
	return m
}

// SetSwallowInput controls whether the trigger button is consumed system-wide.
// When false the hook still triggers the panel but lets the button reach the
// foreground application, so middle-click scrolling and back/forward keep working.
func (m *MouseHookManager) SetSwallowInput(swallow bool) {
	m.swallowInput.Store(swallow)
}

// RehookCount reports how many times the watchdog had to rebuild the hook. It is
// exposed for diagnostics and tests.
func (m *MouseHookManager) RehookCount() uint32 {
	return m.rehookCount.Load()
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
	changed := code != m.activeTrigger.Load()
	m.activeTrigger.Store(code)
	if changed {
		// Drop any swallow state left over from the previous trigger so a
		// pending button-up cannot be eaten by the new configuration.
		m.resetState()
	}

	if code == triggerCodeNone {
		m.stopLocked()
		return nil
	}

	if m.running {
		// Windows silently unhooks low-level hooks whose callback times out.
		// A running manager with no live hhook must be rebuilt, not ignored.
		if m.hhook.Load() != 0 {
			return nil
		}
		m.stopLocked()
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
	m.startWatchdogLocked()
	return nil
}

func (m *MouseHookManager) stopLocked() {
	if !m.running {
		return
	}

	m.stopWatchdogLocked()

	if tid := m.tid.Load(); tid != 0 {
		_ = postThreadMessage(tid, WM_QUIT, 0, 0)
	}
	select {
	case <-m.done:
	case <-time.After(mouseHookStopTimeout):
	}

	close(m.stopWorker)
	select {
	case <-m.workerDone:
	case <-time.After(mouseHookStopTimeout):
	}
	m.hhook.Store(0)
	m.tid.Store(0)

	m.resetState()
	m.running = false
	m.done = nil
	m.stopWorker = nil
	m.workerDone = nil
}

// pressTrigger reports whether the button that just went down is the configured trigger, notifying
// the worker, and returns the hook result (1 to swallow the input) plus whether it applied.
func (m *MouseHookManager) pressTrigger(trigger, code int32, swallowed *atomic.Bool) (uintptr, bool) {
	if trigger != code {
		return 0, false
	}
	m.notifyTrigger()
	if !m.swallowInput.Load() {
		return 0, true
	}
	swallowed.Store(true)
	return 1, true
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
	m.noteEvent()
	if int32(nCode) >= hcAction {
		trigger := m.activeTrigger.Load()

		switch wParam {
		case wmMButtonDown:
			if r, matched := m.pressTrigger(trigger, triggerCodeMiddle, &m.swallowedMiddle); matched {
				return r
			}
		case wmMButtonUp:
			if m.swallowedMiddle.Swap(false) {
				return 1
			}
		case wmXButtonDown:
			if lParam != nil {
				msll := (*msllHookStruct)(lParam)
				xb := uint16(msll.MouseData >> 16)
				var code int32
				swallowed := &m.swallowedX2
				switch xb {
				case xButton1:
					code, swallowed = triggerCodeXButton1, &m.swallowedX1
				case xButton2:
					code, swallowed = triggerCodeXButton2, &m.swallowedX2
				default:
					break
				}
				if r, matched := m.pressTrigger(trigger, code, swallowed); matched {
					return r
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

func (m *MouseHookManager) noteEvent() {
	tick, _, _ := procGetTickCount64.Call()
	if tick != 0 {
		m.lastEventTick.Store(uint64(tick))
	}
}

// startWatchdogLocked spawns the liveness watchdog. The caller holds m.mu only
// for the duration of startLocked, so the goroutine must not touch m.mu while
// waiting; it calls rebuildFromWatchdog which re-acquires it.
func (m *MouseHookManager) startWatchdogLocked() {
	stop := make(chan struct{})
	done := make(chan struct{})
	m.watchdogStop = stop
	m.watchdogDone = done
	m.noteEvent()
	go func() {
		defer close(done)
		ticker := time.NewTicker(mouseHookWatchdogTick)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if !m.hookLooksDead() {
					continue
				}
				m.rebuildFromWatchdog()
			}
		}
	}()
}

// detachWatchdogLocked stops the watchdog goroutine without waiting for it. Only the watchdog's
// own rebuild path calls this: it runs on the goroutine being stopped, so a wait would block on
// its own exit until the stop timeout while holding m.mu.
func (m *MouseHookManager) detachWatchdogLocked() {
	if m.watchdogStop != nil {
		close(m.watchdogStop)
	}
	m.watchdogStop = nil
	m.watchdogDone = nil
}

func (m *MouseHookManager) stopWatchdogLocked() {
	if m.watchdogStop != nil {
		close(m.watchdogStop)
		select {
		case <-m.watchdogDone:
		case <-time.After(mouseHookStopTimeout):
		}
	}
	m.watchdogStop = nil
	m.watchdogDone = nil
}

// inputAgeMillis reports how long ago the last system input happened. GetLastInputInfo reports a
// 32-bit tick count while GetTickCount64 is 64-bit, so the subtraction has to wrap modulo 2^32;
// subtracting the raw values directly broke the check after 49.7 days of uptime.
func inputAgeMillis(now uint64, lastInput uint32) uint32 {
	return uint32(now) - lastInput
}

// hookLooksDead reports true when the system has seen input recently but our
// low-level hook has not been called for longer than mouseHookEventTimeout.
func (m *MouseHookManager) hookLooksDead() bool {
	if m.hhook.Load() == 0 {
		return m.activeTrigger.Load() != triggerCodeNone
	}
	var li lastInputInfo
	li.CbSize = uint32(unsafe.Sizeof(li))
	ok, _, _ := procGetLastInputInfoRef.Call(uintptr(unsafe.Pointer(&li)))
	if ok == 0 {
		return false
	}
	tick, _, _ := procGetTickCount64.Call()
	if tick == 0 {
		return false
	}
	const recentInput = 2500
	if inputAgeMillis(uint64(tick), li.Time) > recentInput {
		return false // nobody is using the mouse, nothing to judge
	}
	last := m.lastEventTick.Load()
	if last == 0 {
		return true
	}
	return uint64(tick)-last > uint64(mouseHookEventTimeout/time.Millisecond)
}

func (m *MouseHookManager) rebuildFromWatchdog() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running || m.activeTrigger.Load() == triggerCodeNone {
		return
	}
	m.detachWatchdogLocked()
	m.stopLocked()
	if err := m.startLocked(); err == nil {
		m.rehookCount.Add(1)
	}
}

func (m *MouseHookManager) notifyTrigger() {
	select {
	case m.triggerCh <- struct{}{}:
	default:
	}
}
