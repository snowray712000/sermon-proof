// Package locate finds the on-disk path of a file the browser knows only by
// name, size and modification time. A page cannot learn the real path of a
// dropped file, so the server identifies it: Safari supplies file:// URIs, every
// other browser gets a Spotlight lookup plus a walk of a few likely folders, and
// a candidate counts only when its size and mtime both match.
package locate

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Query describes the dropped file as the browser reports it.
type Query struct {
	Name    string
	Size    int64
	MtimeMs int64    // File.lastModified
	URIs    []string // text/uri-list entries from the same drop (may be empty)
}

// ErrNotFound means no file matched; the caller should ask for the dialog.
var ErrNotFound = errors.New("找不到這個檔案的位置，請改用「開啟」／「音檔」按鈕")

const walkDepth = 3

// Find returns the one file matching q. dirs are extra folders to walk when the
// faster lookups miss (typically the folder of the transcript already open).
func Find(q Query, dirs []string) (string, error) {
	var found []string
	add := func(p string) {
		if !matches(p, q) {
			return
		}
		for _, f := range found {
			if f == p {
				return
			}
		}
		found = append(found, p)
	}

	for _, u := range q.URIs {
		if p, ok := uriPath(u); ok && filepath.Base(p) == q.Name {
			add(p)
		}
	}
	if len(found) == 0 {
		for _, p := range spotlight(q.Name) {
			add(p)
		}
	}
	if len(found) == 0 {
		for _, d := range dirs {
			walk(d, q.Name, add)
		}
	}

	switch len(found) {
	case 0:
		return "", ErrNotFound
	case 1:
		return found[0], nil
	}
	return "", fmt.Errorf("有多個一模一樣的檔案（%s 等），請改用「開啟」／「音檔」按鈕", found[0])
}

func matches(p string, q Query) bool {
	st, err := os.Stat(p)
	if err != nil || st.IsDir() || st.Size() != q.Size {
		return false
	}
	d := st.ModTime().UnixMilli() - q.MtimeMs
	return d >= -1 && d <= 1
}

func uriPath(u string) (string, bool) {
	pu, err := url.Parse(strings.TrimSpace(u))
	if err != nil || pu.Scheme != "file" || pu.Path == "" {
		return "", false
	}
	p := pu.Path
	if runtime.GOOS == "windows" && len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:] // file:///C:/dir → C:/dir
	}
	return filepath.Clean(p), true
}

func spotlight(name string) []string {
	if runtime.GOOS != "darwin" {
		return nil // mdfind is macOS only; the caller falls back to a folder walk.
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "mdfind", "-name", name).Output()
	if err != nil {
		return nil
	}
	var res []string
	for _, line := range strings.Split(string(out), "\n") {
		if line != "" && filepath.Base(line) == name {
			res = append(res, line)
		}
	}
	return res
}

// walk looks for name under dir and, since transcript and audio often sit in
// sibling folders, under dir's parent as well.
func walk(dir, name string, add func(string)) {
	for _, root := range []string{dir, filepath.Dir(dir)} {
		base := strings.Count(root, string(filepath.Separator))
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return filepath.SkipDir
			}
			if d.IsDir() {
				if strings.Count(p, string(filepath.Separator))-base >= walkDepth {
					return filepath.SkipDir
				}
				return nil
			}
			if d.Name() == name {
				add(p)
			}
			return nil
		})
	}
}
