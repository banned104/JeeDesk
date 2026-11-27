package applog

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// A GUI build has no console, so a crash used to leave nothing behind beyond the log lines written
// before it. This file keeps three kinds of evidence under <baseDir>/logs/crashes:
//
//	runtime-crash-<n>.log  what the Go runtime wrote when it died (debug.SetCrashOutput)
//	<source>-<stamp>.txt   panics we recovered ourselves, with stacks and the log tail
//	frontend-<stamp>.txt   errors reported by the WebView
//
// The name of the newest report is mirrored into last-panic.txt so the next start can point the
// user at it.

const (
	crashDirName   = "crashes"
	runtimeLogName = "runtime-crash"
	maxReports     = 20
	maxLogTail     = 8 << 10
	maxReportBytes = 256 << 10
	// A broken render loop can report the same error thousands of times; keep the first few and
	// count the rest instead of filling the disk.
	maxFrontendPerSignature = 3
	// The total is a per-process backstop, not a per-signature one: a new kind of error still gets
	// reported until this many reports exist.
	maxFrontendReports = 100
	maxRuntimeLogs     = 5
)

var (
	stateMu    sync.RWMutex
	baseDir    string
	appVersion string

	crashed   atomic.Bool
	reportSeq atomic.Uint64

	frontendMu       sync.Mutex
	frontendSeen     = map[string]int{}
	frontendTotal    int
	frontendSuppress int
)

// Init records where crash reports go and which version to stamp them with. It is safe to call more
// than once; the first call wins so an early crash still uses the real data directory.
func Init(dir, version string) {
	stateMu.Lock()
	if baseDir == "" {
		baseDir = dir
	}
	if appVersion == "" {
		appVersion = version
	}
	stateMu.Unlock()
}

func dataDir() string {
	stateMu.RLock()
	dir := baseDir
	stateMu.RUnlock()
	return dir
}

func version() string {
	stateMu.RLock()
	v := appVersion
	stateMu.RUnlock()
	return v
}

// CrashDir returns the folder that holds crash reports for baseDir.
func CrashDir(dir string) string {
	return filepath.Join(dir, "logs", crashDirName)
}

// InstallCrashOutput asks the Go runtime to write fatal errors (nil dereference, invalid fault
// address, a panic that escaped every recover, a throw from a Win32 callback) to a file, because a
// GUI build's stderr goes nowhere. The returned function closes the file on a clean shutdown.
func InstallCrashOutput(dir string) (func(), error) {
	crashDir := CrashDir(dir)
	if err := os.MkdirAll(crashDir, 0o755); err != nil {
		return nil, err
	}
	file, err := openRuntimeCrashLog(crashDir)
	if err != nil {
		return nil, err
	}
	if err := debug.SetCrashOutput(file, debug.CrashOptions{}); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() { _ = file.Close() }, nil
}

// openRuntimeCrashLog opens a fresh file for this process, so a runtime report always belongs to
// exactly one run. Older ones are pruned here rather than only in writeCrashFile: a build that
// never crashes would otherwise leave one empty log per start.
func openRuntimeCrashLog(crashDir string) (*os.File, error) {
	stamp := time.Now().Format("20060102-150405")
	path := filepath.Join(crashDir, fmt.Sprintf("%s-%s-%d.log", runtimeLogName, stamp, os.Getpid()))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	// The file just opened is the newest by modification time, so it survives the prune.
	pruneNewest(crashDir, ".log", maxRuntimeLogs)
	return file, nil
}

// Crashed reports whether this process recorded a crash.
func Crashed() bool { return crashed.Load() }

// Finish clears the run marker, unless something crashed: then it stays behind so the next start
// can tell the user where the report is. It also records how many frontend errors the rate limit
// dropped, because "no report" must not look like "no error".
func Finish(dir string) {
	if suppressed := SuppressedFrontendReports(); suppressed > 0 {
		log.Printf("[crash] %d frontend errors were rate limited; see %s", suppressed, CrashDir(dir))
	}
	if Crashed() {
		return
	}
	ClearMarker(dir)
}

// Recover is meant to be deferred in every goroutine we start. It writes a full report for the
// panic and keeps the run marker, so the failure is visible on the next start instead of ending the
// process without a trace.
//
//	defer applog.Recover("hook.mouse")
func Recover(source string) {
	value := recover()
	if value == nil {
		return
	}
	WriteReport(source, value, debug.Stack())
}

// WriteReport records a recovered panic (or any fatal-looking failure) and returns the path it
// wrote. It never panics: logging a crash must not become a second crash.
func WriteReport(source string, value any, stack []byte) string {
	dir := dataDir()
	if dir == "" {
		// Init never ran: keep at least the standard log line.
		fmt.Fprintf(os.Stderr, "[crash] %s: %v\n", source, value)
		return ""
	}
	return writeCrashFile(dir, source, value, stack, "")
}

// WriteFrontendReport records an error the WebView reported. Identical errors are capped so a broken
// render loop cannot fill the disk.
func WriteFrontendReport(kind, message, stack, detail string) string {
	dir := dataDir()
	if dir == "" {
		return ""
	}
	signature := kind + "\x00" + truncate(message, 200)
	frontendMu.Lock()
	if frontendSeen[signature] >= maxFrontendPerSignature || frontendTotal >= maxFrontendReports {
		frontendSuppress++
		frontendMu.Unlock()
		return ""
	}
	frontendSeen[signature]++
	frontendTotal++
	frontendMu.Unlock()

	body := fmt.Sprintf("kind: %s\nmessage: %s\n", kind, message)
	if detail != "" {
		body += fmt.Sprintf("detail: %s\n", detail)
	}
	return writeCrashFile(dir, "frontend."+kind, message, []byte(stack), body)
}

// SuppressedFrontendReports reports how many frontend errors were dropped by the rate limit, so the
// summary can mention them.
func SuppressedFrontendReports() int {
	frontendMu.Lock()
	defer frontendMu.Unlock()
	return frontendSuppress
}

// writeCrashFile assembles one report. header is extra text placed above the stack.
func writeCrashFile(dir, source string, value any, stack []byte, header string) string {
	crashDir := CrashDir(dir)
	if err := os.MkdirAll(crashDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[crash] %s: cannot create %s: %v\n", source, crashDir, err)
		return ""
	}

	stamp := time.Now()
	// Microseconds plus a per-process counter: two reports of the same source in the same second
	// must not overwrite each other.
	path := filepath.Join(crashDir, fmt.Sprintf("%s-%s-%03d-%d.txt",
		sanitize(source), stamp.Format("20060102-150405.000000"), reportSeq.Add(1), os.Getpid()))

	body := buildReport(dir, source, value, stack, header, stamp)
	if len(body) > maxReportBytes {
		body = body[:maxReportBytes] + "\n[report truncated]\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "[crash] %s: cannot write report: %v\n", source, err)
		return ""
	}

	crashed.Store(true)
	log.Printf("[crash] %s: %v (report: %s)", source, value, path)
	// Keep the newest report name where the next start looks for it.
	_ = os.WriteFile(LatestReportPath(dir), []byte(fmt.Sprintf("%s %s\n%s\n", stamp.Format(time.RFC3339), source, path)), 0o644)
	pruneReports(crashDir)
	return path
}

func buildReport(dir, source string, value any, stack []byte, header string, stamp time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "==== JeeDesk crash report ====\n")
	fmt.Fprintf(&b, "time    : %s\n", stamp.Format(time.RFC3339Nano))
	fmt.Fprintf(&b, "version : %s\n", version())
	fmt.Fprintf(&b, "pid     : %d\n", os.Getpid())
	fmt.Fprintf(&b, "source  : %s\n", source)
	fmt.Fprintf(&b, "runtime : %s %s/%s (%d cpus)\n", runtime.Version(), runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
	fmt.Fprintf(&b, "value   : %v\n", value)
	if header != "" {
		b.WriteString("\n--- detail ---\n")
		b.WriteString(header)
	}
	if len(stack) > 0 {
		b.WriteString("\n--- stack of the failing goroutine ---\n")
		b.Write(stack)
		if stack[len(stack)-1] != '\n' {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n--- all goroutines ---\n")
	writeGoroutineDump(&b)
	// The log tail comes from the same directory the report is written to: taking it from the
	// package-level base dir would silently produce an empty tail whenever they differ.
	if tail := logTail(LogPath(dir), maxLogTail); tail != "" {
		b.WriteString("\n--- tail of logs/app.log ---\n")
		b.WriteString(tail)
		if !strings.HasSuffix(tail, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func writeGoroutineDump(b *strings.Builder) {
	profile := pprof.Lookup("goroutine")
	if profile == nil {
		b.WriteString("[goroutine profile unavailable]\n")
		return
	}
	// debug=1 keeps the dump readable: call stacks without the full raw parameter words.
	_ = profile.WriteTo(writerFunc(func(p []byte) (int, error) {
		if b.Len() > maxReportBytes {
			return len(p), nil
		}
		b.Write(p)
		return len(p), nil
	}), 1)
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// logTail returns the last max bytes of path, which is enough to see what the app was doing.
func logTail(path string, max int64) string {
	if path == "" {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return ""
	}
	offset := int64(0)
	if info.Size() > max {
		offset = info.Size() - max
	}
	if _, err := file.Seek(offset, 0); err != nil {
		return ""
	}
	buf := make([]byte, max)
	n, _ := file.Read(buf)
	return string(buf[:n])
}

// pruneReports keeps the newest reports of each kind so a crash loop cannot fill the disk: the text
// reports, and the runtime's own crash logs (one per start).
func pruneReports(crashDir string) {
	pruneNewest(crashDir, ".txt", maxReports)
	pruneNewest(crashDir, ".log", maxRuntimeLogs)
}

// pruneNewest deletes all but the newest keep files with the given extension.
func pruneNewest(dir, ext string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type report struct {
		path string
		mod  time.Time
	}
	reports := make([]report, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		reports = append(reports, report{path: filepath.Join(dir, entry.Name()), mod: info.ModTime()})
	}
	if len(reports) <= keep {
		return
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].mod.After(reports[j].mod) })
	for _, old := range reports[keep:] {
		_ = os.Remove(old.path)
	}
}

// sanitize turns a source name into something safe to use as a file name.
func sanitize(source string) string {
	if source == "" {
		return "unknown"
	}
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '.' || r == '-' || r == '_':
			return r
		default:
			return '_'
		}
	}, source)
	if len(cleaned) > 60 {
		cleaned = cleaned[:60]
	}
	return cleaned
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
