package locate

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Keep the wide fallback away from the real home folder unless a test opts in.
func init() { wideRoots = func() []string { return nil } }

func mk(t *testing.T, p, body string, mt time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mt, mt); err != nil {
		t.Fatal(err)
	}
}

func TestFindByWalkNeedsSizeAndMtime(t *testing.T) {
	root := t.TempDir()
	mt := time.Unix(1_700_000_000, 123_000_000)
	mk(t, filepath.Join(root, "a", "x.md"), "hello", mt)
	mk(t, filepath.Join(root, "b", "x.md"), "other!", mt) // same name, different size

	got, err := Find(Query{Name: "x.md", Size: 5, MtimeMs: mt.UnixMilli()}, []string{filepath.Join(root, "a")})
	if err != nil || got != filepath.Join(root, "a", "x.md") {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := Find(Query{Name: "x.md", Size: 5, MtimeMs: mt.UnixMilli() + 5000}, []string{filepath.Join(root, "a")}); err != ErrNotFound {
		t.Fatalf("mtime mismatch should not match, got %v", err)
	}
}

func TestFindByURIAndAmbiguity(t *testing.T) {
	root := t.TempDir()
	mt := time.Unix(1_700_000_000, 0)
	p := filepath.Join(root, "講道", "x.md")
	mk(t, p, "hello", mt)
	up := filepath.ToSlash(p)
	if !strings.HasPrefix(up, "/") {
		up = "/" + up // Windows: C:/dir → /C:/dir
	}
	uri := (&url.URL{Scheme: "file", Path: up}).String()
	got, err := Find(Query{Name: "x.md", Size: 5, MtimeMs: mt.UnixMilli(), URIs: []string{uri}}, nil)
	if err != nil || got != p {
		t.Fatalf("got %q, %v", got, err)
	}

	mk(t, filepath.Join(root, "copy", "x.md"), "hello", mt)
	if _, err := Find(Query{Name: "x.md", Size: 5, MtimeMs: mt.UnixMilli()}, []string{filepath.Join(root, "copy")}); err == nil {
		t.Fatal("two identical files must be reported as ambiguous")
	}
}

func TestFindByWideWalkWhenNothingIsOpen(t *testing.T) {
	root := t.TempDir()
	mt := time.Unix(1_700_000_000, 0)
	p := filepath.Join(root, "信望愛", "講道", "x.md")
	mk(t, p, "hello", mt)
	mk(t, filepath.Join(root, ".hidden", "x.md"), "hello", mt) // hidden folders are skipped
	old := wideRoots
	wideRoots = func() []string { return []string{root} }
	defer func() { wideRoots = old }()

	got, err := Find(Query{Name: "x.md", Size: 5, MtimeMs: mt.UnixMilli()}, nil)
	if err != nil || got != p {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := Find(Query{Name: "x.md", Size: 6, MtimeMs: mt.UnixMilli()}, nil); err != ErrNotFound {
		t.Fatalf("size mismatch should not match, got %v", err)
	}
}

func resetRecent(t *testing.T, file string) {
	t.Helper()
	recent.mu.Lock()
	recent.dirs, recent.file = nil, file
	recent.mu.Unlock()
	t.Cleanup(func() {
		recent.mu.Lock()
		recent.dirs, recent.file = nil, ""
		recent.mu.Unlock()
	})
}

func TestFindSearchesRecentFoldersBeforeWideWalk(t *testing.T) {
	root := t.TempDir()
	mt := time.Unix(1_700_000_000, 0)
	p := filepath.Join(root, "a", "b", "c", "d", "e", "f", "g", "x.md") // far too deep for the open-folder walk
	mk(t, p, "hello", mt)
	resetRecent(t, "")
	Remember(filepath.Join(filepath.Dir(p), "y.mp3"))
	old := wideRoots
	wideRoots = func() []string { t.Fatal("wide walk must not run when a recent folder hits"); return nil }
	defer func() { wideRoots = old }()

	got, err := Find(Query{Name: "x.md", Size: 5, MtimeMs: mt.UnixMilli()}, nil)
	if err != nil || got != p {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestRecentListPersistsAndKeepsNewestFirst(t *testing.T) {
	file := filepath.Join(t.TempDir(), "cfg", "recent-dirs.json")
	resetRecent(t, file)
	SetRecentFile(file)
	Remember(filepath.Join("one", "a.md"))
	Remember(filepath.Join("two", "b.md"))
	Remember(filepath.Join("one", "c.mp3")) // moves "one" back to the front
	for i := 0; i < 2*maxRecent; i++ {
		Remember(filepath.Join("many", string(rune('a'+i%26)), string(rune('A'+i/26)), "f"))
	}
	if n := len(recentDirs()); n != maxRecent {
		t.Fatalf("list not capped: %d", n)
	}

	resetRecent(t, "")
	SetRecentFile(file)
	if n := len(recentDirs()); n != maxRecent {
		t.Fatalf("reload lost entries: %d", n)
	}
	resetRecent(t, file)
	Remember(filepath.Join("one", "a.md"))
	Remember(filepath.Join("two", "b.md"))
	Remember(filepath.Join("one", "c.mp3"))
	if d := recentDirs(); len(d) != 2 || d[0] != "one" || d[1] != "two" {
		t.Fatalf("got %v", d)
	}
}

func TestWideWalkReportsCopiesOnDifferentRootsAsAmbiguous(t *testing.T) {
	r1, r2 := t.TempDir(), t.TempDir()
	mt := time.Unix(1_700_000_000, 0)
	mk(t, filepath.Join(r1, "x.md"), "hello", mt)
	mk(t, filepath.Join(r2, "sub", "x.md"), "hello", mt)
	old := wideRoots
	wideRoots = func() []string { return []string{r1, r2} }
	defer func() { wideRoots = old }()

	if _, err := Find(Query{Name: "x.md", Size: 5, MtimeMs: mt.UnixMilli()}, nil); err == nil || err == ErrNotFound {
		t.Fatalf("copies on two roots must be ambiguous, got %v", err)
	}
	// Overlapping roots reach the same file twice; that is still one file.
	wideRoots = func() []string { return []string{r1, filepath.Join(r1, ".")} }
	if got, err := Find(Query{Name: "x.md", Size: 5, MtimeMs: mt.UnixMilli()}, nil); err != nil || got != filepath.Join(r1, "x.md") {
		t.Fatalf("got %q, %v", got, err)
	}
}
