package locate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const maxRecent = 20

// recent remembers the folders of files that were opened before, newest first.
// Dropped files are nearly always in one of them, so searching them first turns
// the multi-second disk walk into a few milliseconds.
var recent struct {
	mu   sync.Mutex
	dirs []string
	file string // "" keeps the list in memory only
}

// SetRecentFile loads the remembered folders from path and saves every later
// change there. Failures are ignored: the list is only a speed-up.
func SetRecentFile(path string) {
	recent.mu.Lock()
	defer recent.mu.Unlock()
	recent.file = path
	var dirs []string
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &dirs) == nil {
		if len(dirs) > maxRecent {
			dirs = dirs[:maxRecent]
		}
		recent.dirs = dirs
	}
}

// Remember records the folder of an opened file.
func Remember(path string) {
	if path == "" {
		return
	}
	dir := filepath.Dir(path)
	recent.mu.Lock()
	defer recent.mu.Unlock()
	dirs := []string{dir}
	for _, d := range recent.dirs {
		if d != dir && len(dirs) < maxRecent {
			dirs = append(dirs, d)
		}
	}
	recent.dirs = dirs
	if recent.file == "" {
		return
	}
	b, err := json.Marshal(dirs)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(recent.file), 0o755) != nil {
		return
	}
	tmp := recent.file + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, recent.file)
	}
}

func recentDirs() []string {
	recent.mu.Lock()
	defer recent.mu.Unlock()
	return append([]string(nil), recent.dirs...)
}
