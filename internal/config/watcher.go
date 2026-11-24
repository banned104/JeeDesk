package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	watchDebounceDelay  = 200 * time.Millisecond
	watchRestartDelay   = time.Second
	watchRestartRetries = 3
	loadRetryAttempts   = 3
	loadRetryDelay      = 60 * time.Millisecond
)

// ErrWriteBlocked is returned by Update while config.json cannot be read or parsed. Saving then
// would replace the file's actual contents with the in-memory copy.
var ErrWriteBlocked = errors.New("config.json 当前无法读取或解析，已阻止写入以保护原文件")

// fileMark identifies one version of the config file on disk.
type fileMark struct {
	size int64
	mod  time.Time
	ok   bool
}

func markOf(path string) fileMark {
	info, err := os.Stat(path)
	if err != nil {
		return fileMark{}
	}
	return fileMark{size: info.Size(), mod: info.ModTime(), ok: true}
}

func (m fileMark) same(other fileMark) bool {
	return m.ok && other.ok && m.size == other.size && m.mod.Equal(other.mod)
}

// Manager manages loading, thread-safe access, atomic updates, and live file watching of config.json.
type Manager struct {
	path    string
	mu      sync.RWMutex
	writeMu sync.Mutex
	current Config

	// diskErr is the last error from reading the config file, or nil while the file is valid.
	// It is only accessed with writeMu held.
	diskErr error

	// mark records the file version the in-memory config was built from, so Update can
	// notice edits made outside the application instead of overwriting them.
	// It is only accessed with writeMu held.
	mark fileMark

	onUpdate  func(Config)
	onError   func(error)
	onRemoved func()

	watchMu sync.Mutex
	watcher *fsnotify.Watcher
	stopCh  chan struct{}
	doneCh  chan struct{}
}

// NewManager loads or creates the config file at configPath and initializes a Manager.
func NewManager(configPath string, onUpdate func(Config), onError func(error)) (*Manager, error) {
	cleaned := filepath.Clean(configPath)
	cfg, err := LoadOrCreate(cleaned)
	if err != nil {
		return nil, err
	}

	return &Manager{
		path:     cleaned,
		current:  Clone(cfg),
		mark:     markOf(cleaned),
		onUpdate: onUpdate,
		onError:  onError,
	}, nil
}

// NewManagerWithFallback always returns an initialized Manager capable of watching configPath.
// If configPath exists with invalid JSON, the Manager starts with DefaultConfig in memory
// without overwriting the broken file and returns the parse error alongside the live Manager.
func NewManagerWithFallback(configPath string, onUpdate func(Config), onError func(error)) (*Manager, error) {
	cleaned := filepath.Clean(configPath)
	cfg, err := LoadOrCreate(cleaned)
	if err != nil {
		return &Manager{
			path:     cleaned,
			current:  DefaultConfig(),
			diskErr:  err,
			onUpdate: onUpdate,
			onError:  onError,
		}, err
	}

	return &Manager{
		path:     cleaned,
		current:  Clone(cfg),
		mark:     markOf(cleaned),
		onUpdate: onUpdate,
		onError:  onError,
	}, nil
}

// Path returns the managed configuration file path.
func (m *Manager) Path() string {
	return m.path
}

// SetRemovedHandler installs a callback invoked when config.json disappears from disk.
func (m *Manager) SetRemovedHandler(fn func()) {
	m.mu.Lock()
	m.onRemoved = fn
	m.mu.Unlock()
}

// Settings returns a copy of the current settings. It is much cheaper than Get and is what the
// window-visibility poller reads several times per second.
func (m *Manager) Settings() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current.Settings
}

// Get returns a deep copy of the current valid in-memory configuration.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Clone(m.current)
}

// Update serializes writes, applies mutate to a cloned config, normalizes it, and saves to disk.
// Before writing it verifies the file on disk is still the version memory was built from; if it
// changed, the file is re-read first so edits made outside the application are preserved. While
// the file cannot be read or parsed, Update refuses to save and returns ErrWriteBlocked.
func (m *Manager) Update(mutate func(*Config) error) error {
	return m.update(mutate, true)
}

// UpdateQuiet applies the same mutation without notifying onUpdate. It exists for changes the UI
// already knows about, such as the window position, where a full config:updated round trip would
// re-render the whole window on every drag.
func (m *Manager) UpdateQuiet(mutate func(*Config) error) error {
	return m.update(mutate, false)
}

func (m *Manager) update(mutate func(*Config) error, notify bool) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	base, err := m.syncLocked()
	if err != nil {
		return err
	}

	next := Clone(base)
	if mutate != nil {
		if err := mutate(&next); err != nil {
			return err
		}
	}
	Normalize(&next)

	// Only a structural change is worth a backup; window geometry is written every few hundred
	// milliseconds and would otherwise replace the .bak with a copy of the current file.
	if err := SaveWithBackup(m.path, next, needsBackup(base, next)); err != nil {
		return err
	}

	m.mark = markOf(m.path)
	m.diskErr = nil

	m.mu.Lock()
	m.current = Clone(next)
	m.mu.Unlock()

	// Notify immediately: the UI must not depend on the filesystem watcher, which can
	// fail to start or miss events.
	if notify && m.onUpdate != nil {
		m.onUpdate(Clone(next))
	}
	return nil
}

// syncLocked reconciles memory with the file on disk. The caller holds writeMu.
func (m *Manager) syncLocked() (Config, error) {
	if m.diskErr != nil {
		// A previous read failed. Retry so a transient lock (antivirus, editor) heals
		// itself instead of blocking writes for the rest of the session.
		cfg, err := loadWithRetry(m.path)
		if err != nil {
			m.diskErr = err
			return Config{}, fmt.Errorf("%w（%v）", ErrWriteBlocked, err)
		}
		m.diskErr = nil
		m.mark = markOf(m.path)
		m.mu.Lock()
		m.current = Clone(cfg)
		m.mu.Unlock()
		return cfg, nil
	}

	now := markOf(m.path)
	if !now.ok || now.same(m.mark) {
		return m.Get(), nil
	}

	// Somebody edited the file behind our back: adopt their version, then apply the
	// caller's mutation on top of it.
	cfg, err := loadWithRetry(m.path)
	if err != nil {
		m.diskErr = err
		return Config{}, fmt.Errorf("%w（%v）", ErrWriteBlocked, err)
	}
	m.mark = now
	m.mu.Lock()
	m.current = Clone(cfg)
	m.mu.Unlock()
	return cfg, nil
}

// Reload reads and parses config.json. On success it updates memory and calls onUpdate.
// On error it preserves the last valid in-memory config, calls onError, and returns the error.
// The error is also kept as the reason to block writes until a later load succeeds.
func (m *Manager) Reload() (Config, error) {
	return m.reload(false)
}

// ReloadForce reads the file even when it did not change on disk and always notifies. The tray and
// settings "reload" actions use it so the UI is refreshed on demand.
func (m *Manager) ReloadForce() (Config, error) {
	return m.reload(true)
}

func (m *Manager) reload(force bool) (Config, error) {
	m.writeMu.Lock()
	previousMark := m.mark
	cfg, err := loadWithRetry(m.path)
	unchanged := err == nil && !force && previousMark.ok && previousMark.same(markOf(m.path))
	if err == nil {
		m.mark = markOf(m.path)
		m.mu.Lock()
		m.current = Clone(cfg)
		m.mu.Unlock()
	}
	m.diskErr = err
	m.writeMu.Unlock()

	if err != nil {
		if m.onError != nil {
			m.onError(err)
		}
		return Config{}, err
	}

	// A reload triggered by our own Save must not re-render the whole window.
	if m.onUpdate != nil && !unchanged {
		m.onUpdate(Clone(cfg))
	}
	return Clone(cfg), nil
}

// loadWithRetry retries transient filesystem errors. Parse errors are returned at once:
// retrying cannot fix a malformed file.
func loadWithRetry(path string) (Config, error) {
	var err error
	for attempt := 0; attempt < loadRetryAttempts; attempt++ {
		var cfg Config
		cfg, err = Load(path)
		if err == nil {
			return cfg, nil
		}
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			return Config{}, err
		}
		if attempt < loadRetryAttempts-1 {
			time.Sleep(loadRetryDelay)
		}
	}
	return Config{}, err
}

// StartWatching starts watching the config file's parent directory with a 200ms debounce.
func (m *Manager) StartWatching() error {
	m.watchMu.Lock()
	defer m.watchMu.Unlock()

	if m.watcher != nil {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	dir := filepath.Dir(m.path)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return err
	}

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	m.watcher = watcher
	m.stopCh = stopCh
	m.doneCh = doneCh

	go m.watchLoop(watcher, stopCh, doneCh, filepath.Base(m.path))
	return nil
}

// Close stops the debounce timer and closes the filesystem watcher cleanly.
func (m *Manager) Close() error {
	m.watchMu.Lock()
	watcher := m.watcher
	stopCh := m.stopCh
	doneCh := m.doneCh
	m.watcher = nil
	m.stopCh = nil
	m.doneCh = nil
	m.watchMu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}

	var closeErr error
	if watcher != nil {
		closeErr = watcher.Close()
	}

	if doneCh != nil {
		<-doneCh
	}

	return closeErr
}

// needsBackup reports whether a write changes something a user would want back: groups or any
// setting other than window geometry and the folded-group list.
func needsBackup(before, after Config) bool {
	if !reflect.DeepEqual(before.Groups, after.Groups) {
		return true
	}
	left, right := before.Settings, after.Settings
	left.WindowX, left.WindowY, left.WindowWidth, left.WindowHeight = 0, 0, 0, 0
	right.WindowX, right.WindowY, right.WindowWidth, right.WindowHeight = 0, 0, 0, 0
	left.CollapsedGroups, right.CollapsedGroups = nil, nil
	return !reflect.DeepEqual(left, right)
}

// confirmRemoved waits out the short window in which a replacing write looks like a deletion and
// reports whether the configuration file is really gone. It stats the watched path itself: the
// watch loop's targetBase is the file name, not its directory.
func (m *Manager) confirmRemoved() bool {
	time.Sleep(150 * time.Millisecond)
	_, err := os.Stat(m.path)
	return errors.Is(err, os.ErrNotExist)
}

func (m *Manager) watchLoop(watcher *fsnotify.Watcher, stopCh <-chan struct{}, doneCh chan<- struct{}, targetBase string) {
	var timer *time.Timer
	var timerC <-chan time.Time

	defer func() {
		if timer != nil {
			timer.Stop()
		}
		close(doneCh)
	}()

	for {
		select {
		case <-stopCh:
			return
		case event, ok := <-watcher.Events:
			if !ok {
				m.restartWatching()
				return
			}
			if isConfigRemoval(event, targetBase) {
				// A save that replaces the file also surfaces as a removal on Windows, so the
				// handler only runs when the path is really gone after the write settles.
				if !m.confirmRemoved() {
					continue
				}
				m.mu.RLock()
				handler := m.onRemoved
				m.mu.RUnlock()
				if handler != nil {
					handler()
				}
				continue
			}
			if !isRelevantConfigEvent(event, targetBase) {
				continue
			}
			if timer == nil {
				timer = time.NewTimer(watchDebounceDelay)
				timerC = timer.C
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(watchDebounceDelay)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				m.restartWatching()
				return
			}
			// Watch errors are about the notification channel, not about config.json
			// contents, so they must not be reported as a config parse failure.
			if err != nil {
				log.Printf("config watch: %v", err)
			}
		case <-timerC:
			timer = nil
			timerC = nil
			_, _ = m.Reload()
		}
	}
}

// restartWatching recreates the watcher after its channels were closed by the OS.
func (m *Manager) restartWatching() {
	for attempt := 0; attempt < watchRestartRetries; attempt++ {
		time.Sleep(watchRestartDelay)
		if err := m.reopenWatcher(); err == nil {
			return
		}
	}
	log.Printf("config watch: giving up after %d restart attempts", watchRestartRetries)
}

func (m *Manager) reopenWatcher() error {
	m.watchMu.Lock()
	m.watcher = nil
	m.stopCh = nil
	m.doneCh = nil
	m.watchMu.Unlock()
	return m.StartWatching()
}

func isConfigRemoval(event fsnotify.Event, targetBase string) bool {
	if !event.Has(fsnotify.Remove) {
		return false
	}
	return baseMatches(event.Name, targetBase)
}

func isRelevantConfigEvent(event fsnotify.Event, targetBase string) bool {
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
		return false
	}
	return baseMatches(event.Name, targetBase)
}

func baseMatches(name, targetBase string) bool {
	base := filepath.Base(name)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(base, targetBase)
	}
	return base == targetBase
}
