package applog

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteReportCapturesTheContext is the reason this file exists: a report has to be usable
// without a debugger, so it must name the source, the value, the stack and the log tail.
func TestWriteReportCapturesTheContext(t *testing.T) {
	dir := t.TempDir()
	Init(dir, "9.9.9")
	logPath, closeLog, err := Setup(dir)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer closeLog()
	log.Printf("[test] app.log tail marker")

	path := WriteReport("hook.mouse", "boom", []byte("goroutine 1 [running]:\nmain.foo()\n\tapp.go:1\n"))
	if path == "" {
		t.Fatal("WriteReport returned an empty path")
	}
	_ = logPath

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	report := string(data)
	for _, want := range []string{"source  : hook.mouse", "value   : boom", "main.foo()", "version : 9.9.9"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report is missing %q:\n%s", want, report)
		}
	}
	if !strings.Contains(report, "--- all goroutines ---") {
		t.Fatalf("report has no goroutine dump:\n%s", report)
	}
	if !Crashed() {
		t.Fatal("Crashed() = false after a report was written")
	}

	// The pointer file the next start reads must name the report just written.
	pointer, err := os.ReadFile(LatestReportPath(dir))
	if err != nil {
		t.Fatalf("reading the report pointer failed: %v", err)
	}
	if !strings.Contains(string(pointer), path) {
		t.Fatalf("last-panic pointer %q does not name %q", pointer, path)
	}
}

// TestReportsOfTheSameSourceDoNotCollide matters because the frontend allows three reports of the
// same error: a second-resolution name would silently overwrite the first.
func TestReportsOfTheSameSourceDoNotCollide(t *testing.T) {
	dir := t.TempDir()
	Init(dir, "1.0.0")

	first := WriteReport("frontend.promise", "same error", nil)
	second := WriteReport("frontend.promise", "same error", nil)
	if first == second {
		t.Fatalf("two reports share the path %q", first)
	}
	for _, path := range []string{first, second} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("report %q missing: %v", path, err)
		}
	}
}

// TestFrontendReportsAreRateLimited keeps a broken render loop from filling the disk, while still
// telling the caller how many errors it dropped.
func TestFrontendReportsAreRateLimited(t *testing.T) {
	dir := t.TempDir()
	Init(dir, "1.0.0")

	written := 0
	for i := 0; i < maxFrontendPerSignature+5; i++ {
		if path := WriteFrontendReport("vue", "render failed", "stack", "component: Grid"); path != "" {
			written++
		}
	}
	if written != maxFrontendPerSignature {
		t.Fatalf("wrote %d reports for one signature, want %d", written, maxFrontendPerSignature)
	}
	if suppressed := SuppressedFrontendReports(); suppressed != 5 {
		t.Fatalf("SuppressedFrontendReports() = %d, want 5", suppressed)
	}

	// A different error is not what the per-signature limit is for.
	if path := WriteFrontendReport("vue", "another failure", "", ""); path == "" {
		t.Fatal("a new signature was suppressed by the per-signature limit")
	}
}

// TestFinishKeepsTheMarkerAfterACrash pins the difference between a clean exit and a reported
// crash: only the clean one may delete the run marker.
func TestFinishKeepsTheMarkerAfterACrash(t *testing.T) {
	dir := t.TempDir()
	Init(dir, "1.0.0")
	if err := WriteMarker(dir); err != nil {
		t.Fatalf("WriteMarker failed: %v", err)
	}

	WriteReport("test", "boom", nil)
	Finish(dir)
	if !PreviousRunCrashed(dir) {
		t.Fatal("Finish cleared the marker of a crashed run")
	}

	crashed.Store(false)
	Finish(dir)
	if PreviousRunCrashed(dir) {
		t.Fatal("Finish kept the marker of a clean run")
	}
}

// TestPruneReportsBoundsBothKinds keeps a crash loop from filling the disk with either our reports
// or the runtime's own crash logs.
func TestPruneReportsBoundsBothKinds(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(CrashDir(dir), 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	for i := 0; i < maxReports+5; i++ {
		path := filepath.Join(CrashDir(dir), "old-"+string(rune('a'+i%26))+"-"+itoa(i)+".txt")
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}
	for i := 0; i < maxRuntimeLogs+3; i++ {
		path := filepath.Join(CrashDir(dir), runtimeLogName+"-"+itoa(i)+".log")
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}

	pruneReports(CrashDir(dir))

	texts, logs := 0, 0
	entries, err := os.ReadDir(CrashDir(dir))
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	for _, entry := range entries {
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".txt":
			texts++
		case ".log":
			logs++
		}
	}
	if texts > maxReports {
		t.Fatalf("%d text reports left, want at most %d", texts, maxReports)
	}
	if logs > maxRuntimeLogs {
		t.Fatalf("%d runtime logs left, want at most %d", logs, maxRuntimeLogs)
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
