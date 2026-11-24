package tray

import _ "embed"

// trayIconICO is the JeeDesk blade mark as a 32x32 32-bit ICO. It is a file rather than drawn in
// code so the tray, the window, the executable, and the installed shortcut all show the same icon.
// Regenerate it from assets/appicon.png with _scratch tooling when the artwork changes.
//
//go:embed tray.ico
var trayIconICO []byte

// IconBytes returns a fresh copy of the tray icon so the caller may hand it to the system without
// aliasing the embedded data.
func IconBytes() []byte {
	out := make([]byte, len(trayIconICO))
	copy(out, trayIconICO)
	return out
}
