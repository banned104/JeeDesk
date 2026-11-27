package main

import (
	"embed"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"jeedesk/internal/applog"
	"jeedesk/internal/config"
	"jeedesk/internal/resolver"
)

//go:embed all:frontend/dist
var assets embed.FS

const (
	minWindowWidth  = 580
	minWindowHeight = 380
)

func main() {
	baseDir, dataDirNotice := resolveBaseDirWithFallback()

	// Crash evidence has to be installable before anything else can panic: a GUI build has no
	// stderr, so the runtime's own crash output is the only trace a fatal fault would leave.
	applog.Init(baseDir, appVersion)
	if closeCrashLog, err := applog.InstallCrashOutput(baseDir); err != nil {
		log.Printf("[crash] cannot install runtime crash output: %v", err)
	} else {
		defer closeCrashLog()
	}
	// A panic before the run marker exists is rare but would otherwise leave nothing behind. This
	// one is registered first, so it runs last: when the Recover below already handled the panic it
	// sees nil and does nothing.
	defer applog.Recover("main.early")

	app := NewApp(baseDir)

	// A pre-upgrade build uses the old fixed mutex name; wake it rather than running a second copy
	// that would write the same config.json.
	if legacyInstanceRunning() {
		return
	}

	// The single-instance mutex is taken before the config is opened: LoadOrCreate writes a
	// default config.json when the file is missing, and a second launch doing that while the
	// primary runs would replace the user's groups with defaults.
	mutexName, className, windowName := SingleInstanceNames(baseDir)
	si, isPrimary, err := AcquireSingleInstance(mutexName, className, windowName, app.ShowWindow)
	if err != nil {
		log.Printf("[single-instance] acquire failed: %v", err)
	} else if !isPrimary {
		return
	}
	if si != nil {
		defer si.Close()
	}

	// A GUI build has no console, so the log file is the only trace of a failed startup. It is
	// opened after the primary check so a second launch cannot rotate the log the primary owns.
	if path, closeLog, logErr := applog.Setup(baseDir); logErr != nil {
		log.Printf("[app] cannot open log file: %v", logErr)
	} else {
		defer closeLog()
		log.Printf("[app] JeeDesk %s starting, base dir %s, log %s", appVersion, baseDir, path)
	}

	// A marker left behind by the previous run means it never shut down cleanly. Only the primary
	// instance owns it: a second launch that wakes the running copy must not clear it.
	app.previousRunCrashed = applog.PreviousRunCrashed(baseDir)
	app.startupNotice = dataDirNotice
	// Load the config before anything can run concurrently with OnStartup.
	app.loadConfig()
	_ = applog.WriteMarker(baseDir)
	// Finish clears the marker only when nothing crashed. It must be registered before the Recover
	// below: defers run last-in-first-out, so the report has to be written first, otherwise Finish
	// still sees the flag clear and deletes the marker of a crashed run.
	defer applog.Finish(baseDir)
	defer applog.Recover("main")

	// The window is created once, so the saved size, theme, and start-hidden preference have to
	// be read here rather than applied later from the frontend.
	bootCfg := app.GetConfig()
	width, height, startHidden, theme := bootWindowOptions(bootCfg.Settings)
	if silentLaunch() {
		startHidden = true
	}
	app.startHidden = startHidden

	err = wails.Run(&options.App{
		Title:            "JeeDesk",
		Width:            width,
		Height:           height,
		MinWidth:         minWindowWidth,
		MinHeight:        minWindowHeight,
		Frameless:        true,
		StartHidden:      startHidden,
		AlwaysOnTop:      true,
		BackgroundColour: &options.RGBA{R: 18, G: 22, B: 32, A: 0},
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: newIconMiddleware(app.iconsDir, app.customIconRoots),
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
			CSSDropProperty:    "--wails-drop-target",
			CSSDropValue:       "drop",
		},
		OnStartup:     app.startup,
		OnDomReady:    app.domReady,
		OnBeforeClose: app.beforeClose,
		OnShutdown:    app.shutdown,
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			// Keep WebView2's profile next to config.json so a portable folder copies as one unit.
			WebviewUserDataPath:  filepath.Join(baseDir, "webview"),
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         windows.Acrylic,
			DisableWindowIcon:    true,
			Theme:                theme,
		},
	})
	if err != nil {
		log.Printf("[app] JeeDesk failed to start: %v", err)
		applog.RecordPanic(baseDir, err)
		os.Exit(1)
	}
}

// bootWindowOptions turns saved settings into the window options Wails needs at startup.
// A saved size is reused only when it is large enough to stay usable, and a theme without an
// explicit preference follows the Windows app theme.
func bootWindowOptions(settings config.Settings) (width, height int, startHidden bool, theme windows.Theme) {
	width, height = config.DefaultWindowWidth, config.DefaultWindowHeight
	if settings.WindowWidth >= minWindowWidth {
		width = settings.WindowWidth
	}
	if settings.WindowHeight >= minWindowHeight {
		height = settings.WindowHeight
	}

	switch settings.Theme {
	case config.ThemeLight:
		theme = windows.Light
	case config.ThemeSystem:
		theme = windows.SystemDefault
	default:
		theme = windows.Dark
	}

	return width, height, settings.StartHidden, theme
}

// silentLaunch reports whether the executable was started with --silent, which the autostart entry
// and a shortcut can use to start without showing the panel.
func silentLaunch() bool {
	for _, arg := range os.Args[1:] {
		if strings.EqualFold(strings.TrimSpace(arg), "--silent") {
			return true
		}
	}
	return false
}

// resolveBaseDirWithFallback returns a writable data directory. A read-only location (a USB stick
// with the write-protect switch on, a Program Files install without rights) falls back to
// %LOCALAPPDATA%\JeeDesk and reports why, instead of failing on every save.
func resolveBaseDirWithFallback() (string, string) {
	baseDir := resolveBaseDir()
	if isWritableDir(baseDir) {
		return baseDir, ""
	}
	fallback := filepath.Join(os.Getenv("LOCALAPPDATA"), "JeeDesk")
	if isWritableDir(fallback) {
		return fallback, fmt.Sprintf("数据目录 %s 不可写，已改用 %s", baseDir, fallback)
	}
	return baseDir, ""
}

// isWritableDir probes a directory by creating and removing a temporary file.
func isWritableDir(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	probe, err := os.CreateTemp(dir, ".write-probe-*")
	if err != nil {
		return false
	}
	name := probe.Name()
	_ = probe.Close()
	_ = os.Remove(name)
	return true
}

// resolveBaseDir picks the directory that holds config.json and icons/.
// JEEDESK_BASE_DIR overrides the automatic choice, which is what a portable copy or a test run
// with its own data directory needs.
func resolveBaseDir() string {
	if dir := strings.TrimSpace(os.Getenv("JEEDESK_BASE_DIR")); dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			return abs
		}
		return dir
	}

	cwd, cwdErr := os.Getwd()
	exePath, exeErr := os.Executable()
	return pickBaseDir(cwd, cwdErr == nil, exePath, exeErr == nil)
}

// pickBaseDir prefers a working directory that already holds JeeDesk's config.json. Otherwise the
// executable's directory is used, except in development (go run, wails dev), where the working
// directory is used.
func pickBaseDir(cwd string, cwdOK bool, exePath string, exeOK bool) string {
	if !cwdOK {
		cwd = ""
	}
	// An explicit marker beats guessing from the binary path: JEEDESK_DEV=1 for a hand-started
	// developer run, and the assetdir variable wails dev sets for the process it launches.
	if cwd != "" && devLaunch() {
		return cwd
	}
	if cwd != "" && config.LooksLikeConfig(filepath.Join(cwd, "config.json")) {
		return cwd
	}

	if exeOK {
		exeDir := filepath.Dir(exePath)
		if !isDevBinaryDir(exeDir, cwd) {
			return exeDir
		}
	}

	if cwd != "" {
		return cwd
	}
	return "."
}

// devLaunch reports an explicitly marked development run.
func devLaunch() bool {
	if strings.TrimSpace(os.Getenv("JEEDESK_DEV")) == "1" {
		return true
	}
	return strings.TrimSpace(os.Getenv("assetdir")) != ""
}

// isDevBinaryDir reports whether exeDir is where go run or wails dev place the binary: Go's build
// cache under the user's temp directory, or the project's own build/bin. It is the fallback for
// runs without a marker; devLaunch handles wails dev and JEEDESK_DEV=1 first.
func isDevBinaryDir(exeDir, cwd string) bool {
	lowerExe := strings.ToLower(exeDir)
	if strings.Contains(lowerExe, `\appdata\local\temp\`) ||
		strings.Contains(lowerExe, `\go-build`) {
		return true
	}
	return cwd != "" && strings.EqualFold(exeDir, filepath.Join(cwd, "build", "bin"))
}

var allowedCustomIconExts = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
	".gif":  true,
	".bmp":  true,
	".svg":  true,
	".ico":  true,
}

// customIconRoots are the only trees the custom icon endpoint reads from: the data directory, the
// user's profile, and the folders configured items point at. Item icons are extracted from those
// targets, so a portable config on D: keeps working without opening the whole disk to the endpoint.
func (a *App) customIconRoots() []string {
	roots := []string{a.baseDir}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		roots = append(roots, home)
	}
	for _, group := range a.GetConfig().Groups {
		for _, item := range group.Items {
			if dir := filepath.Dir(strings.TrimSpace(item.Path)); dir != "" && dir != "." {
				roots = append(roots, dir)
			}
			if icon := strings.TrimSpace(item.Icon); icon != "" && !strings.HasPrefix(icon, "icons/") {
				if dir := filepath.Dir(icon); dir != "" && dir != "." {
					roots = append(roots, dir)
				}
			}
		}
	}
	return roots
}

// pathWithinRoots reports whether path is one of roots or below it, case-insensitively.
func pathWithinRoots(path string, roots []string) bool {
	cleaned := strings.ToLower(filepath.Clean(path))
	for _, root := range roots {
		base := strings.ToLower(filepath.Clean(root))
		if base == "" || base == "." {
			continue
		}
		if cleaned == base || strings.HasPrefix(cleaned, base+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

// newIconMiddleware serves cached icons from /icons/<file> and user-specified
// custom icons from /custom-icon?path=<encoded-path>.
func newIconMiddleware(iconsDir string, customRoots func() []string) assetserver.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/icons/"):
				serveCachedIcon(w, r, iconsDir)
				return
			case r.URL.Path == "/custom-icon":
				serveCustomIcon(w, r, iconsDir, customRoots())
				return
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
}

// safeCustomIconPath keeps the custom icon endpoint to ordinary local files: an absolute path with
// no device or UNC prefix, so a crafted URL cannot reach a named pipe or a network share.
func safeCustomIconPath(rawPath string) bool {
	if !filepath.IsAbs(rawPath) {
		return false
	}
	if strings.HasPrefix(rawPath, `\\`) || strings.HasPrefix(rawPath, `//`) {
		return false
	}
	return filepath.VolumeName(rawPath) != ""
}

func serveCachedIcon(w http.ResponseWriter, r *http.Request, iconsDir string) {
	rel := strings.TrimPrefix(r.URL.Path, "/icons/")
	if rel == "" || strings.ContainsAny(rel, `/\`) || rel == "." || rel == ".." {
		http.NotFound(w, r)
		return
	}

	safeName := filepath.Base(rel)
	if safeName != rel {
		http.NotFound(w, r)
		return
	}

	fullPath := filepath.Join(iconsDir, safeName)
	info, err := os.Stat(fullPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	if ct := mime.TypeByExtension(filepath.Ext(fullPath)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeFile(w, r, fullPath)
}

func serveCustomIcon(w http.ResponseWriter, r *http.Request, iconsDir string, customRoots []string) {
	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if rawPath == "" || !safeCustomIconPath(rawPath) || !pathWithinRoots(rawPath, customRoots) {
		http.NotFound(w, r)
		return
	}

	cleaned := filepath.Clean(rawPath)
	ext := strings.ToLower(filepath.Ext(cleaned))

	// An image the user picked as an icon is served as-is.
	if allowedCustomIconExts[ext] {
		if info, err := os.Stat(cleaned); err == nil && !info.IsDir() {
			w.Header().Set("Cache-Control", "no-cache")
			if ct := mime.TypeByExtension(ext); ct != "" {
				w.Header().Set("Content-Type", ct)
			}
			http.ServeFile(w, r, cleaned)
			return
		}
	}

	// Everything else is a launch target: .exe, .dll, .bat, .lnk, .html, a folder or a path
	// without an extension. Those get the same shell icon Explorer would show, cached as PNG.
	relIcon, err := resolver.ExtractAndCacheIcon(cleaned, iconsDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "image/png")
	http.ServeFile(w, r, filepath.Join(iconsDir, filepath.Base(relIcon)))
}
