package hook

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32 hotkey and message constants.
const (
	MOD_ALT      uint32 = 0x0001
	MOD_CONTROL  uint32 = 0x0002
	MOD_SHIFT    uint32 = 0x0004
	MOD_WIN      uint32 = 0x0008
	MOD_NOREPEAT uint32 = 0x4000

	WM_HOTKEY uint32 = 0x0312
	WM_QUIT   uint32 = 0x0012
	WM_APP    uint32 = 0x8000
)

const (
	pmNoRemove     uintptr = 0x0000
	wmUpdateHotkey uint32  = WM_APP + 1
	hotkeyID       uintptr = 1
)

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey      = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey    = user32.NewProc("UnregisterHotKey")
	procPeekMessageW        = user32.NewProc("PeekMessageW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
)

type winPOINT struct {
	X int32
	Y int32
}

type winMSG struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      winPOINT
}

var namedKeys = map[string]uint32{
	"space":     0x20,
	"tab":       0x09,
	"enter":     0x0D,
	"return":    0x0D,
	"esc":       0x1B,
	"escape":    0x1B,
	"backspace": 0x08,
	"up":        0x26,
	"down":      0x28,
	"left":      0x25,
	"right":     0x27,
	"home":      0x24,
	"end":       0x23,
	"insert":    0x2D,
	"delete":    0x2E,
	"pageup":    0x21,
	"pagedown":  0x22,
	"`":         0xC0,
	"backquote": 0xC0,
	"oem3":      0xC0,
	"-":         0xBD,
	"minus":     0xBD,
	"=":         0xBB,
	"equal":     0xBB,
	"[":         0xDB,
	"]":         0xDD,
	`\`:         0xDC,
	";":         0xBA,
	"'":         0xDE,
	",":         0xBC,
	".":         0xBE,
	"/":         0xBF,
}

// ParseHotkey parses a hotkey specification such as "Alt+G" or "Ctrl+Shift+Space"
// into Win32 modifier flags (always including MOD_NOREPEAT) and a virtual-key code.
func ParseHotkey(spec string) (modifiers uint32, vk uint32, err error) {
	trimmed := strings.TrimSpace(spec)
	if trimmed == "" {
		return 0, 0, errors.New("empty hotkey specification")
	}

	parts := strings.Split(trimmed, "+")
	var hasKey bool

	for _, raw := range parts {
		token := strings.ToLower(strings.TrimSpace(raw))
		if token == "" {
			return 0, 0, fmt.Errorf("invalid empty token in hotkey %q", spec)
		}

		switch token {
		case "alt":
			modifiers |= MOD_ALT
			continue
		case "ctrl", "control":
			modifiers |= MOD_CONTROL
			continue
		case "shift":
			modifiers |= MOD_SHIFT
			continue
		case "win", "super", "meta":
			modifiers |= MOD_WIN
			continue
		}

		parsedVK, ok := parseKeyToken(token)
		if !ok {
			return 0, 0, fmt.Errorf("unknown hotkey token %q in %q", strings.TrimSpace(raw), spec)
		}
		if hasKey {
			return 0, 0, fmt.Errorf("multiple keys in hotkey %q", spec)
		}
		vk = parsedVK
		hasKey = true
	}

	if !hasKey {
		return 0, 0, fmt.Errorf("missing main key in hotkey %q", spec)
	}

	modifiers |= MOD_NOREPEAT
	return modifiers, vk, nil
}

func parseKeyToken(token string) (uint32, bool) {
	if len(token) == 1 {
		ch := token[0]
		if ch >= 'a' && ch <= 'z' {
			return uint32(ch - 'a' + 'A'), true
		}
		if ch >= '0' && ch <= '9' {
			return uint32(ch), true
		}
	}

	if strings.HasPrefix(token, "f") && len(token) >= 2 && len(token) <= 3 {
		if n, err := strconv.Atoi(token[1:]); err == nil && n >= 1 && n <= 24 {
			return uint32(0x70 + n - 1), true
		}
	}

	vk, ok := namedKeys[token]
	return vk, ok
}

type hotkeyUpdateReq struct {
	spec string
	resp chan error
}

// HotkeyManager manages a global Windows hotkey on a dedicated OS-locked thread.
type HotkeyManager struct {
	onTrigger func()

	mu      sync.Mutex
	started bool
	tid     uint32
	done    chan struct{}
	reqCh   chan *hotkeyUpdateReq
}

// NewHotkeyManager creates a HotkeyManager that invokes onTrigger asynchronously
// whenever the registered hotkey is pressed.
func NewHotkeyManager(onTrigger func()) *HotkeyManager {
	return &HotkeyManager{
		onTrigger: onTrigger,
		reqCh:     make(chan *hotkeyUpdateReq, 1),
	}
}

// Start spawns the dedicated OS-locked message loop thread and registers spec if non-empty.
func (h *HotkeyManager) Start(spec string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return h.updateLocked(spec)
	}
	return h.startLocked(spec)
}

func (h *HotkeyManager) startLocked(spec string) error {
	if spec != "" {
		if _, _, err := ParseHotkey(spec); err != nil {
			return err
		}
	}

	// Drain any stale pending request before starting.
	select {
	case <-h.reqCh:
	default:
	}

	initDone := make(chan error, 1)
	done := make(chan struct{})
	tidReady := make(chan uint32, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(done)

		// Force creation of this thread's message queue before accepting PostThreadMessageW.
		var msg winMSG
		procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, pmNoRemove)
		tid := windows.GetCurrentThreadId()

		var activeSpec string
		var registered bool

		if spec != "" {
			mods, vk, err := ParseHotkey(spec)
			if err != nil {
				initDone <- err
				return
			}
			if err := registerHotKey(hotkeyID, mods, vk); err != nil {
				initDone <- err
				return
			}
			activeSpec = spec
			registered = true
		}

		tidReady <- tid
		initDone <- nil

		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			r := int32(ret)
			if r == 0 || r == -1 {
				if registered {
					unregisterHotKey(hotkeyID)
					registered = false
				}
				return
			}

			switch msg.Message {
			case WM_HOTKEY:
				if msg.WParam == hotkeyID && h.onTrigger != nil {
					go h.onTrigger()
				}
			case wmUpdateHotkey:
				var req *hotkeyUpdateReq
				select {
				case req = <-h.reqCh:
				default:
				}
				if req == nil {
					continue
				}
				err := applyHotkeyUpdate(req.spec, &activeSpec, &registered)
				req.resp <- err
			case WM_QUIT:
				if registered {
					unregisterHotKey(hotkeyID)
					registered = false
				}
				return
			}
		}
	}()

	if err := <-initDone; err != nil {
		<-done
		return err
	}

	h.tid = <-tidReady
	h.done = done
	h.started = true
	return nil
}

func applyHotkeyUpdate(newSpec string, activeSpec *string, registered *bool) error {
	if newSpec == "" {
		if *registered {
			unregisterHotKey(hotkeyID)
			*registered = false
		}
		*activeSpec = ""
		return nil
	}

	mods, vk, err := ParseHotkey(newSpec)
	if err != nil {
		return err
	}

	if *registered {
		unregisterHotKey(hotkeyID)
		*registered = false
	}

	if err := registerHotKey(hotkeyID, mods, vk); err != nil {
		if *activeSpec != "" {
			if prevMods, prevVK, pErr := ParseHotkey(*activeSpec); pErr == nil {
				if rErr := registerHotKey(hotkeyID, prevMods, prevVK); rErr == nil {
					*registered = true
				}
			}
		}
		return err
	}

	*activeSpec = newSpec
	*registered = true
	return nil
}

// Update changes the active hotkey safely from any goroutine.
// If the manager has not been started yet, Update starts it with spec.
func (h *HotkeyManager) Update(spec string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started {
		return h.startLocked(spec)
	}
	return h.updateLocked(spec)
}

func (h *HotkeyManager) updateLocked(spec string) error {
	if spec != "" {
		if _, _, err := ParseHotkey(spec); err != nil {
			return err
		}
	}

	req := &hotkeyUpdateReq{
		spec: spec,
		resp: make(chan error, 1),
	}
	h.reqCh <- req

	if err := postThreadMessage(h.tid, wmUpdateHotkey, 0, 0); err != nil {
		select {
		case <-h.reqCh:
		default:
		}
		return err
	}

	select {
	case err := <-req.resp:
		return err
	case <-h.done:
		h.started = false
		h.tid = 0
		h.done = nil
		return errors.New("hotkey message loop exited unexpectedly")
	}
}

// Stop unregisters any active hotkey and shuts down the message loop thread.
// Stop is idempotent.
func (h *HotkeyManager) Stop() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started {
		return
	}

	_ = postThreadMessage(h.tid, WM_QUIT, 0, 0)
	<-h.done

	h.started = false
	h.tid = 0
	h.done = nil
}

func registerHotKey(id uintptr, modifiers, vk uint32) error {
	r1, _, err := procRegisterHotKey.Call(0, id, uintptr(modifiers), uintptr(vk))
	if r1 == 0 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return fmt.Errorf("RegisterHotKey failed: %w", err)
		}
		return errors.New("RegisterHotKey failed")
	}
	return nil
}

func unregisterHotKey(id uintptr) {
	procUnregisterHotKey.Call(0, id)
}

func postThreadMessage(tid uint32, msg uint32, wParam, lParam uintptr) error {
	r1, _, err := procPostThreadMessageW.Call(uintptr(tid), uintptr(msg), wParam, lParam)
	if r1 == 0 {
		if err != nil && err != windows.ERROR_SUCCESS {
			return fmt.Errorf("PostThreadMessageW failed: %w", err)
		}
		return errors.New("PostThreadMessageW failed")
	}
	return nil
}
