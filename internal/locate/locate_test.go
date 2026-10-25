package locate

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
	uri := (&url.URL{Scheme: "file", Path: p}).String()
	got, err := Find(Query{Name: "x.md", Size: 5, MtimeMs: mt.UnixMilli(), URIs: []string{uri}}, nil)
	if err != nil || got != p {
		t.Fatalf("got %q, %v", got, err)
	}

	mk(t, filepath.Join(root, "copy", "x.md"), "hello", mt)
	if _, err := Find(Query{Name: "x.md", Size: 5, MtimeMs: mt.UnixMilli()}, []string{filepath.Join(root, "copy")}); err == nil {
		t.Fatal("two identical files must be reported as ambiguous")
	}
}
