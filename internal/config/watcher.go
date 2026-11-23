package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const watchDebounceDelay = 200 * time.Millisecond

// ErrWriteBlocked is returned by Update while config.json cannot be read or parsed. Saving then
// would replace the file's actual contents with the in-memory copy.
var ErrWriteBlocked = errors.New("config.json 当前无法读取或解析，已阻止写入以保护原文件")

// Manager manages loading, thread-safe access, atomic updates, and live file watching of config.json.
type Manager struct {
	path    string
	mu      sync.RWMutex
	writeMu sync.Mutex
	current Config

	// diskErr is the last error from reading the config file, or nil while the file is valid.
	// It is only accessed with writeMu held.
	diskErr error

	onUpdate func(Config)
	onError  func(error)

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
		onUpdate: onUpdate,
		onError:  onError,
	}, nil
}

// Path returns the managed configuration file path.
func (m *Manager) Path() string {
	return m.path
}

// Get returns a deep copy of the current valid in-memory configuration.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Clone(m.current)
}

// Update serializes writes, applies mutate to a cloned config, normalizes it, and saves to disk.
// In-memory state is kept consistent, while external/UI notifications flow through Reload via fsnotify.
// While the file on disk cannot be read or parsed, Update refuses to save and returns ErrWriteBlocked.
func (m *Manager) Update(mutate func(*Config) error) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	if m.diskErr != nil {
		return fmt.Errorf("%w（%v）", ErrWriteBlocked, m.diskErr)
	}

	next := m.Get()
	if mutate != nil {
		if err := mutate(&next); err != nil {
			return err
		}
	}
	Normalize(&next)

	if err := Save(m.path, next); err != nil {
		return err
	}

	m.mu.Lock()
	m.current = Clone(next)
	m.mu.Unlock()

	return nil
}

// Reload reads and parses config.json. On success it updates memory and calls onUpdate.
// On error it preserves the last valid in-memory config, calls onError, and returns the error.
// The error is also kept as the reason to block writes until a later load succeeds.
func (m *Manager) Reload() (Config, error) {
	m.writeMu.Lock()
	cfg, err := Load(m.path)
	if err == nil {
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

	if m.onUpdate != nil {
		m.onUpdate(Clone(cfg))
	}
	return Clone(cfg), nil
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
				return
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
				return
			}
			if err != nil && m.onError != nil {
				m.onError(err)
			}
		case <-timerC:
			timer = nil
			timerC = nil
			_, _ = m.Reload()
		}
	}
}

func isRelevantConfigEvent(event fsnotify.Event, targetBase string) bool {
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
		return false
	}
	base := filepath.Base(event.Name)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(base, targetBase)
	}
	return base == targetBase
}
