package usage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"jeedesk/internal/applog"
)

const (
	// flushDelay batches launches so a burst of clicks writes the file once.
	flushDelay = 800 * time.Millisecond
	// maxEntries bounds the file: keeping every historical item forever has no value.
	maxEntries = 500
)

// Entry records how often and how recently one item was launched.
type Entry struct {
	GroupID  string `json:"group_id"`
	ItemID   string `json:"item_id"`
	Count    int    `json:"count"`
	LastUsed int64  `json:"last_used"` // unix milliseconds
}

type store struct {
	Entries []Entry `json:"entries"`
}

// Store keeps launch counters in a small JSON file next to config.json. It never blocks the UI:
// writes are debounced and a failure is visible through LastError.
type Store struct {
	path string

	mu      sync.Mutex
	entries map[string]Entry
	lastErr error
	timer   *time.Timer
	closed  bool
}

// NewStore loads path, or starts empty when the file is missing or unreadable. A damaged file is
// ignored instead of failing startup: usage data is a convenience, not a source of truth.
func NewStore(path string) *Store {
	s := &Store{
		path:    filepath.Clean(path),
		entries: make(map[string]Entry),
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return s
	}
	var payload store
	if err := json.Unmarshal(data, &payload); err != nil {
		return s
	}
	for _, entry := range payload.Entries {
		if entry.GroupID == "" || entry.ItemID == "" {
			continue
		}
		s.entries[key(entry.GroupID, entry.ItemID)] = entry
	}
	return s
}

func key(groupID, itemID string) string {
	return groupID + "\x00" + itemID
}

// Record counts one launch of the item.
func (s *Store) Record(groupID, itemID string) {
	if groupID == "" || itemID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}

	entry := s.entries[key(groupID, itemID)]
	entry.GroupID = groupID
	entry.ItemID = itemID
	entry.Count++
	entry.LastUsed = time.Now().UnixMilli()
	s.entries[key(groupID, itemID)] = entry

	s.scheduleFlushLocked()
}

// Forget drops the record of one item, used when the item is deleted.
func (s *Store) Forget(groupID, itemID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[key(groupID, itemID)]; !ok {
		return
	}
	delete(s.entries, key(groupID, itemID))
	s.scheduleFlushLocked()
}

// ForgetGroup drops every record of a deleted group.
func (s *Store) ForgetGroup(groupID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := false
	for k, entry := range s.entries {
		if entry.GroupID == groupID {
			delete(s.entries, k)
			removed = true
		}
	}
	if removed {
		s.scheduleFlushLocked()
	}
}

// Prune removes records of items that no longer exist. keep maps groupID to a set of item IDs.
func (s *Store) Prune(keep map[string]map[string]bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := false
	for k, entry := range s.entries {
		if !keep[entry.GroupID][entry.ItemID] {
			delete(s.entries, k)
			removed = true
		}
	}
	if removed {
		s.scheduleFlushLocked()
	}
}

// Snapshot returns the entries sorted by most recent launch first.
func (s *Store) Snapshot() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastUsed != out[j].LastUsed {
			return out[i].LastUsed > out[j].LastUsed
		}
		return out[i].Count > out[j].Count
	})
	return out
}

// Count returns how often an item was launched.
func (s *Store) Count(groupID, itemID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entries[key(groupID, itemID)].Count
}

// LastError reports the most recent write failure, if any.
func (s *Store) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *Store) scheduleFlushLocked() {
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(flushDelay, func() {
		defer applog.Recover("usage.flushTimer")
		_ = s.Flush()
	})
}

// Flush writes the file now. Callers get the error as well as it being kept for LastError.
func (s *Store) Flush() error {
	s.mu.Lock()
	payload := store{Entries: make([]Entry, 0, len(s.entries))}
	for _, entry := range s.entries {
		payload.Entries = append(payload.Entries, entry)
	}
	path := s.path
	s.mu.Unlock()

	sort.Slice(payload.Entries, func(i, j int) bool {
		return payload.Entries[i].LastUsed > payload.Entries[j].LastUsed
	})
	if len(payload.Entries) > maxEntries {
		payload.Entries = payload.Entries[:maxEntries]
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err == nil {
		data = append(data, '\n')
		err = writeAtomic(path, data)
	}

	s.mu.Lock()
	s.lastErr = err
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("写入使用记录失败: %w", err)
	}
	return nil
}

// Close stops the pending flush and writes what is left.
func (s *Store) Close() error {
	s.mu.Lock()
	s.closed = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	return s.Flush()
}

func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".usage-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// ErrUnavailable marks a store that could not be created.
var ErrUnavailable = errors.New("使用记录不可用")
