package hook

import (
	"runtime"
	"testing"
	"time"
)

func TestParseHotkeyValid(t *testing.T) {
	tests := []struct {
		spec     string
		wantMods uint32
		wantVK   uint32
	}{
		{"Alt+G", MOD_ALT | MOD_NOREPEAT, 'G'},
		{"Ctrl+Shift+Space", MOD_CONTROL | MOD_SHIFT | MOD_NOREPEAT, 0x20},
		{"Win+F12", MOD_WIN | MOD_NOREPEAT, 0x7B},
		{"Backquote", MOD_NOREPEAT, 0xC0},
		{"Alt+`", MOD_ALT | MOD_NOREPEAT, 0xC0},
		{"Oem3", MOD_NOREPEAT, 0xC0},
		{"  cTrL + aLt + sHiFt + f1  ", MOD_CONTROL | MOD_ALT | MOD_SHIFT | MOD_NOREPEAT, 0x70},
		{"Control+Super+Meta+Return", MOD_CONTROL | MOD_WIN | MOD_NOREPEAT, 0x0D},
		{"Ctrl+1", MOD_CONTROL | MOD_NOREPEAT, '1'},
		{"F24", MOD_NOREPEAT, 0x87},
		{"Esc", MOD_NOREPEAT, 0x1B},
		{"Escape", MOD_NOREPEAT, 0x1B},
		{"Tab", MOD_NOREPEAT, 0x09},
		{"Enter", MOD_NOREPEAT, 0x0D},
		{"Backspace", MOD_NOREPEAT, 0x08},
		{"Up", MOD_NOREPEAT, 0x26},
		{"Down", MOD_NOREPEAT, 0x28},
		{"Left", MOD_NOREPEAT, 0x25},
		{"Right", MOD_NOREPEAT, 0x27},
		{"Home", MOD_NOREPEAT, 0x24},
		{"End", MOD_NOREPEAT, 0x23},
		{"Insert", MOD_NOREPEAT, 0x2D},
		{"Delete", MOD_NOREPEAT, 0x2E},
		{"PageUp", MOD_NOREPEAT, 0x21},
		{"PageDown", MOD_NOREPEAT, 0x22},
		{"Ctrl+-", MOD_CONTROL | MOD_NOREPEAT, 0xBD},
		{"Ctrl+Minus", MOD_CONTROL | MOD_NOREPEAT, 0xBD},
		{"Ctrl+=", MOD_CONTROL | MOD_NOREPEAT, 0xBB},
		{"Ctrl+Equal", MOD_CONTROL | MOD_NOREPEAT, 0xBB},
		{"Ctrl+[", MOD_CONTROL | MOD_NOREPEAT, 0xDB},
		{"Ctrl+]", MOD_CONTROL | MOD_NOREPEAT, 0xDD},
		{`Ctrl+\`, MOD_CONTROL | MOD_NOREPEAT, 0xDC},
		{"Ctrl+;", MOD_CONTROL | MOD_NOREPEAT, 0xBA},
		{"Ctrl+'", MOD_CONTROL | MOD_NOREPEAT, 0xDE},
		{"Ctrl+,", MOD_CONTROL | MOD_NOREPEAT, 0xBC},
		{"Ctrl+.", MOD_CONTROL | MOD_NOREPEAT, 0xBE},
		{"Ctrl+/", MOD_CONTROL | MOD_NOREPEAT, 0xBF},
	}

	for _, tc := range tests {
		mods, vk, err := ParseHotkey(tc.spec)
		if err != nil {
			t.Errorf("ParseHotkey(%q) unexpected error: %v", tc.spec, err)
			continue
		}
		if mods != tc.wantMods || vk != tc.wantVK {
			t.Errorf("ParseHotkey(%q) = (0x%X, 0x%X), want (0x%X, 0x%X)",
				tc.spec, mods, vk, tc.wantMods, tc.wantVK)
		}
	}
}

func TestParseHotkeyInvalid(t *testing.T) {
	invalids := []string{
		"",
		"   ",
		"Ctrl+",
		"+G",
		"Ctrl++G",
		"Ctrl+Alt",
		"Shift+Win",
		"Ctrl+A+B",
		"Ctrl+UnknownKey",
		"F0",
		"F25",
		"Alt+Foo",
	}

	for _, spec := range invalids {
		mods, vk, err := ParseHotkey(spec)
		if err == nil {
			t.Errorf("ParseHotkey(%q) succeeded with (0x%X, 0x%X), want error", spec, mods, vk)
		}
	}
}

func TestValidateMouseTrigger(t *testing.T) {
	valid := []struct {
		in   string
		want string
	}{
		{"none", MouseTriggerNone},
		{"NONE", MouseTriggerNone},
		{"  Middle  ", MouseTriggerMiddle},
		{"XButton1", MouseTriggerXButton1},
		{"xbutton2", MouseTriggerXButton2},
	}
	for _, tc := range valid {
		got, err := ValidateMouseTrigger(tc.in)
		if err != nil {
			t.Errorf("ValidateMouseTrigger(%q) unexpected error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ValidateMouseTrigger(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	invalids := []string{"", "   ", "left", "right", "xbutton3", "wheel"}
	for _, in := range invalids {
		if got, err := ValidateMouseTrigger(in); err == nil {
			t.Errorf("ValidateMouseTrigger(%q) = %q, want error", in, got)
		}
	}
}

func TestHotkeyManagerLifecycle(t *testing.T) {
	baseGoroutines := runtime.NumGoroutine()

	hm := NewHotkeyManager(func() {})
	defer hm.Stop()

	if err := hm.Start("Ctrl+Alt+Shift+F20"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if err := hm.Update("Ctrl+Alt+Shift+F21"); err != nil {
		t.Fatalf("Update to F21 failed: %v", err)
	}
	if err := hm.Update("invalid+key"); err == nil {
		t.Fatal("Update with invalid spec unexpectedly succeeded")
	}
	if err := hm.Update(""); err != nil {
		t.Fatalf("Update to empty failed: %v", err)
	}
	if err := hm.Update("Ctrl+Alt+Shift+F22"); err != nil {
		t.Fatalf("Update to F22 failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		hm.Stop()
		hm.Stop() // verify idempotency
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("HotkeyManager.Stop timed out")
	}

	// Allow runtime scheduler to reclaim finished goroutines.
	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > baseGoroutines {
		t.Errorf("goroutine count increased from %d to %d after Stop", baseGoroutines, after)
	}
}

func TestHotkeyManagerUpdateBeforeStart(t *testing.T) {
	hm := NewHotkeyManager(func() {})
	defer hm.Stop()

	if err := hm.Update("Ctrl+Alt+Shift+F23"); err != nil {
		t.Fatalf("Update before Start failed: %v", err)
	}
	hm.Stop()
}

func TestHotkeyManagerTriggerAndExclusivity(t *testing.T) {
	const spec = "Ctrl+Alt+Shift+F20"
	mods, vk, err := ParseHotkey(spec)
	if err != nil {
		t.Fatalf("ParseHotkey(%q) failed: %v", spec, err)
	}

	triggered := make(chan struct{}, 4)
	hm := NewHotkeyManager(func() {
		select {
		case triggered <- struct{}{}:
		default:
		}
	})
	defer hm.Stop()

	if err := hm.Start(spec); err != nil {
		t.Fatalf("Start(%q) failed: %v", spec, err)
	}

	// RegisterHotKey exclusivity must hold while HotkeyManager is running.
	if err := registerHotKey(99, mods, vk); err == nil {
		unregisterHotKey(99)
		t.Fatal("expected RegisterHotKey to fail while HotkeyManager holds the hotkey")
	}

	// Non-matching hotkey ID should not invoke onTrigger.
	if err := postThreadMessage(hm.tid, WM_HOTKEY, hotkeyID+1, 0); err != nil {
		t.Fatalf("postThreadMessage(non-matching) failed: %v", err)
	}
	select {
	case <-triggered:
		t.Fatal("did not expect onTrigger for non-matching WM_HOTKEY wParam")
	case <-time.After(60 * time.Millisecond):
	}

	// Posting WM_HOTKEY with hotkeyID should fire onTrigger cleanly.
	if err := postThreadMessage(hm.tid, WM_HOTKEY, hotkeyID, 0); err != nil {
		t.Fatalf("postThreadMessage(hotkeyID) failed: %v", err)
	}
	select {
	case <-triggered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for onTrigger after WM_HOTKEY")
	}

	hm.Stop()

	// Exclusivity must be released once Stop() completes.
	if err := registerHotKey(99, mods, vk); err != nil {
		t.Fatalf("expected RegisterHotKey to succeed after Stop(), got: %v", err)
	}
	unregisterHotKey(99)
}

func TestMouseHookManagerLifecycle(t *testing.T) {
	baseGoroutines := runtime.NumGoroutine()

	mm := NewMouseHookManager(func() {})
	defer mm.Stop()

	if err := mm.Update("invalid"); err == nil {
		t.Fatal("Update with invalid trigger unexpectedly succeeded")
	}
	if err := mm.Update(MouseTriggerNone); err != nil {
		t.Fatalf("Update(none) failed: %v", err)
	}
	if err := mm.Update(MouseTriggerXButton2); err != nil {
		t.Fatalf("Update(xbutton2) failed: %v", err)
	}
	if err := mm.Update(MouseTriggerMiddle); err != nil {
		t.Fatalf("Update(middle) failed: %v", err)
	}
	if err := mm.Update(MouseTriggerXButton1); err != nil {
		t.Fatalf("Update(xbutton1) failed: %v", err)
	}
	if err := mm.Update(MouseTriggerNone); err != nil {
		t.Fatalf("Update(none) after active failed: %v", err)
	}

	// Re-enable and then stop directly.
	if err := mm.Update(MouseTriggerXButton2); err != nil {
		t.Fatalf("Update(xbutton2) second time failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		mm.Stop()
		mm.Stop() // verify idempotency
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("MouseHookManager.Stop timed out")
	}

	time.Sleep(50 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > baseGoroutines {
		t.Errorf("goroutine count increased from %d to %d after Stop", baseGoroutines, after)
	}
}
