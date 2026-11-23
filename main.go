package main

import (
	"embed"
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

	"jeedesk/internal/config"
	"jeedesk/internal/resolver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	baseDir := resolveBaseDir()
	app := NewApp(baseDir)

	si, isPrimary, err := AcquireSingleInstance(
		singleInstanceMutexName,
		singleInstanceClassName,
		singleInstanceWindowName,
		app.ShowWindow,
	)
	if err != nil {
		log.Printf("[single-instance] acquire failed: %v", err)
	} else if !isPrimary {
		return
	}
	if si != nil {
		defer si.Close()
	}

	err = wails.Run(&options.App{
		Title:            "JeeDesk",
		Width:            800,
		Height:           520,
		MinWidth:         580,
		MinHeight:        380,
		Frameless:        true,
		StartHidden:      false,
		AlwaysOnTop:      true,
		BackgroundColour: &options.RGBA{R: 18, G: 22, B: 32, A: 0},
		AssetServer: &assetserver.Options{
			Assets:     assets,
			Middleware: newIconMiddleware(app.iconsDir),
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
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         windows.Acrylic,
			DisableWindowIcon:    true,
			Theme:                windows.Dark,
		},
	})
	if err != nil {
		log.Fatalf("JeeDesk failed to start: %v", err)
	}
}

// resolveBaseDir picks the directory that holds config.json and icons/.
func resolveBaseDir() string {
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

// isDevBinaryDir reports whether exeDir is where go run or wails dev place the binary: Go's build
// cache under the user's temp directory, or the project's own build/bin. wails dev runs that binary
// with the project root as working directory, so cwd is the project root; it is empty when the
// working directory could not be determined, which leaves only the temp-directory check.
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

// newIconMiddleware serves cached icons from /icons/<file> and user-specified
// custom icons from /custom-icon?path=<encoded-path>.
func newIconMiddleware(iconsDir string) assetserver.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasPrefix(r.URL.Path, "/icons/"):
				serveCachedIcon(w, r, iconsDir)
				return
			case r.URL.Path == "/custom-icon":
				serveCustomIcon(w, r, iconsDir)
				return
			default:
				next.ServeHTTP(w, r)
			}
		})
	}
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

func serveCustomIcon(w http.ResponseWriter, r *http.Request, iconsDir string) {
	rawPath := strings.TrimSpace(r.URL.Query().Get("path"))
	if rawPath == "" {
		http.NotFound(w, r)
		return
	}

	cleaned := filepath.Clean(rawPath)
	ext := strings.ToLower(filepath.Ext(cleaned))

	// If user points custom icon to an executable/dll, extract & serve its cached PNG.
	if ext == ".exe" || ext == ".dll" {
		relIcon, err := resolver.ExtractAndCacheIcon(cleaned, iconsDir)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		filename := filepath.Base(relIcon)
		fullPath := filepath.Join(iconsDir, filename)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "image/png")
		http.ServeFile(w, r, fullPath)
		return
	}

	if !allowedCustomIconExts[ext] {
		http.Error(w, "unsupported icon format", http.StatusForbidden)
		return
	}

	info, err := os.Stat(cleaned)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-cache")
	if ct := mime.TypeByExtension(ext); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeFile(w, r, cleaned)
}
