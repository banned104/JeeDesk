// Package applog redirects the standard logger into a file next to config.json so a GUI build,
// which has no console, still leaves a trace, and keeps a crash marker between runs.
package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	// MaxSize is the size at which app.log is rotated to app.log.1.
	MaxSize = 2 << 20
	logDir  = "logs"
	logName = "app.log"
	marker  = ".running"
)

// Setup opens <baseDir>/logs/app.log (rotating it first when it grew past MaxSize) and sends the
// standard logger to both the file and stderr, so `wails dev` keeps its console output. The
// returned function closes the file.
func Setup(baseDir string) (string, func(), error) {
	dir := filepath.Join(baseDir, logDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	path := filepath.Join(dir, logName)

	if info, err := os.Stat(path); err == nil && info.Size() > MaxSize {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", nil, err
	}

	// The file goes first and stderr is wrapped: a GUI build has no valid stderr handle, and
	// io.MultiWriter stops at the first error, which used to swallow every line.
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetOutput(io.MultiWriter(file, bestEffortWriter{inner: os.Stderr}))
	return path, func() { _ = file.Close() }, nil
}

// bestEffortWriter keeps a broken console from silencing the log file.
type bestEffortWriter struct {
	inner io.Writer
}

func (w bestEffortWriter) Write(p []byte) (int, error) {
	_, _ = w.inner.Write(p)
	return len(p), nil
}

// LogPath returns where Setup would put the log file for baseDir.
func LogPath(baseDir string) string {
	return filepath.Join(baseDir, logDir, logName)
}

// PanicPath returns the file that keeps the last recovered panic for the next start.
func PanicPath(baseDir string) string {
	return filepath.Join(baseDir, logDir, "last-panic.txt")
}

// MarkerPath returns the file whose presence means the previous run never finished.
func MarkerPath(baseDir string) string {
	return filepath.Join(baseDir, logDir, marker)
}

// PreviousRunCrashed reports whether the marker from an earlier run is still on disk.
func PreviousRunCrashed(baseDir string) bool {
	_, err := os.Stat(MarkerPath(baseDir))
	return err == nil
}

// WriteMarker records that a run has started.
func WriteMarker(baseDir string) error {
	path := MarkerPath(baseDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(time.Now().Format(time.RFC3339)), 0o644)
}

// ClearMarker removes the crash marker after a clean shutdown.
func ClearMarker(baseDir string) {
	_ = os.Remove(MarkerPath(baseDir))
}

// RecordPanic logs a recovered panic and keeps it for the next start.
func RecordPanic(baseDir string, recovered any) {
	msg := fmt.Sprintf("[panic] %v", recovered)
	log.Print(msg)
	if path := filepath.Join(baseDir, "logs", "last-panic.txt"); path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			_ = os.WriteFile(path, []byte(fmt.Sprintf("%s %s\n", time.Now().Format(time.RFC3339), msg)), 0o644)
		}
	}
}
