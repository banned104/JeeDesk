//go:build ignore

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"jeedesk/internal/tray"
)

func main() {
	out := filepath.Join("build", "windows", "icon.ico")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		panic(err)
	}
	data := tray.IconBytes()
	if err := os.WriteFile(out, data, 0o644); err != nil {
		panic(err)
	}
	fmt.Printf("generated %s (%d bytes)\n", out, len(data))
}
