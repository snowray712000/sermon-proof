package transcript

import (
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func roundTrip(t *testing.T, name string, in string) {
	t.Helper()
	if out := Parse(in).Render(); out != in {
		t.Fatalf("%s: round-trip differs (len in=%d out=%d)", name, len(in), len(out))
	}
}

func TestRoundTripTestdata(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/*.md")
	if len(files) == 0 {
		t.Fatal("no testdata")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip(t, f, string(b))
	}
}

// SERMON_PROOF_CORPUS=/path/to/folder runs the round-trip over every .md there.
func TestRoundTripCorpus(t *testing.T) {
	dir := os.Getenv("SERMON_PROOF_CORPUS")
	if dir == "" {
		t.Skip("SERMON_PROOF_CORPUS not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		roundTrip(t, f, string(b))
	}
	t.Logf("%d files", len(files))
}

func TestRoundTripEdgeCases(t *testing.T) {
	cases := map[string]string{
		"empty":       "",
		"no trailing": "**[00:00.000]** a",
		"bom":         "\xef\xbb\xbf# t\n\n**[00:01.000]** a\n",
		"mixed eol":   "# t\r\n\n**[00:01.000]** a\r\n\r\n[00:02.000] b\n",
		"only blank":  "\n\n\n",
		"lone cr":     "x\ry\n**[00:01.000]** a\rb\n",
		"no sep":      "**[00:01.000]**\n",
		"fullwidth":   "**[00:01.000]**　a\n",
		"hours":       "[1:02:03.004] a\n",
		"table":       "| a | b |\n| - | - |\n| 00:01.000 | x |\n",
		"unknown":     "random\n\n[not a time] x\n",
	}
	for n, c := range cases {
		roundTrip(t, n, c)
	}
}

func TestRoundTripRandom(t *testing.T) {
	pieces := []string{"**[00:01.000]**", "[02:03.456]", " ", "　", "文字", "，", "[疑似：詞，00:05.420]", "\n", "\r\n", "\r", "#", "|", "- ", "...", "……", "\xef\xbb\xbf"}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 3000; i++ {
		var sb strings.Builder
		for j := rng.Intn(30); j > 0; j-- {
			sb.WriteString(pieces[rng.Intn(len(pieces))])
		}
		roundTrip(t, "random", sb.String())
	}
}

const sample = "# 標題\r\n\r\n- 說明\r\n\r\n## 一、逐字稿\r\n\r\n**[00:00.000]** 第一段[疑似：詞，00:05.420]……\r\n\r\n**[00:32.780]** 第二段\r\n\r\n## 二、清單\r\n\r\n| a | b |\r\n| 00:05.420 | 詞 |\r\n"

func TestParseSegments(t *testing.T) {
	d := Parse(sample)
	if d.NumSegments() != 2 || d.EOL != "\r\n" {
		t.Fatalf("segments=%d eol=%q", d.NumSegments(), d.EOL)
	}
	s := d.Segment(1)
	if s.StartMs != 32780 || s.Time != "00:32.780" || s.Body != "第二段" || s.EOL != "\r\n" {
		t.Fatalf("bad segment %+v", s)
	}
	if got := d.Segment(0).Body; got != "第一段[疑似：詞，00:05.420]……" {
		t.Fatalf("body %q", got)
	}
}

func TestPlainPrefixAndContinuation(t *testing.T) {
	in := "[00:01.000] a\nb\n\n[00:02.000] c\n# h\n"
	d := Parse(in)
	if d.NumSegments() != 2 {
		t.Fatalf("segments=%d", d.NumSegments())
	}
	if d.Segment(0).Body != "a\nb" {
		t.Fatalf("continuation not joined: %q", d.Segment(0).Body)
	}
	if d.Segment(1).Body != "c" {
		t.Fatalf("heading swallowed: %q", d.Segment(1).Body)
	}
}

func TestEditChangesOnlyThatLine(t *testing.T) {
	d := Parse(sample)
	n, err := d.Apply(map[int]string{1: "第二段改過\n新行"})
	if err != nil || n != 1 {
		t.Fatalf("apply n=%d err=%v", n, err)
	}
	want := strings.Replace(sample, "**[00:32.780]** 第二段\r\n", "**[00:32.780]** 第二段改過\r\n新行\r\n", 1)
	if got := d.Render(); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestApplyUnchangedIsNoop(t *testing.T) {
	d := Parse(sample)
	n, _ := d.Apply(map[int]string{0: "第一段[疑似：詞，00:05.420]……", 1: "第二段"})
	if n != 0 || d.Render() != sample {
		t.Fatalf("n=%d", n)
	}
	if _, err := d.Apply(map[int]string{9: "x"}); err == nil {
		t.Fatal("expected error for bad id")
	}
}

func TestEditLastLineWithoutEOL(t *testing.T) {
	d := Parse("**[00:00.000]** a")
	d.Apply(map[int]string{0: "b"})
	if got := d.Render(); got != "**[00:00.000]** b" {
		t.Fatalf("got %q", got)
	}
}

func TestPairAudio(t *testing.T) {
	dir := t.TempDir()
	touch := func(n string) { os.WriteFile(filepath.Join(dir, n), nil, 0o644) }
	touch("N59_1_12_1_13.mp3")
	touch("N59_x-ver1.mp3")
	for _, c := range []struct{ md, want string }{
		{"N59_1_12_1_13-ver1.md", "N59_1_12_1_13.mp3"},
		{"N59_1_12_1_13-ver1-manual.md", "N59_1_12_1_13.mp3"},
		{"N59_1_12_1_13-ver12.orig.md", "N59_1_12_1_13.mp3"},
		{"N59_1_12_1_13.md", "N59_1_12_1_13.mp3"},
		{"N59_x-ver1-manual.md", "N59_x-ver1.mp3"},
	} {
		got, ok := PairAudio(filepath.Join(dir, c.md))
		if !ok || filepath.Base(got) != c.want {
			t.Errorf("%s -> %q %v, want %s", c.md, got, ok, c.want)
		}
	}
	if _, ok := PairAudio(filepath.Join(dir, "other-ver1.md")); ok {
		t.Error("unexpected pair")
	}
	if !IsOrigName("a.orig.md") || IsOrigName("a.md") {
		t.Error("IsOrigName")
	}
}

func TestSaveBackupAtomicAndConflict(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a-ver1.md")
	orig := "**[00:00.000]** a\n"
	os.WriteFile(p, []byte(orig), 0o640)

	h, err := Save(p, []byte("**[00:00.000]** b\n"), Hash([]byte(orig)))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(OrigPath(p)); string(b) != orig {
		t.Fatalf("backup = %q", b)
	}
	if b, _ := os.ReadFile(p); string(b) != "**[00:00.000]** b\n" || Hash(b) != h {
		t.Fatalf("file = %q", b)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o640 {
		t.Fatalf("mode %v", st.Mode())
	}

	// second save must not overwrite the backup
	if _, err := Save(p, []byte("c\n"), h); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(OrigPath(p)); string(b) != orig {
		t.Fatalf("backup overwritten: %q", b)
	}

	// stale hash -> conflict, file untouched
	if _, err := Save(p, []byte("d\n"), h); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "c\n" {
		t.Fatalf("file changed on conflict: %q", b)
	}

	// no temp files left behind
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover %s", e.Name())
		}
	}
}

func TestReviewRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.md")
	if r := LoadReview(p); len(r.Segments) != 0 {
		t.Fatal("expected empty")
	}
	r := &Review{Segments: map[string]bool{"0": true}}
	if err := SaveReview(p, r); err != nil {
		t.Fatal(err)
	}
	got := LoadReview(p)
	if !got.Segments["0"] || got.Suspects == nil {
		t.Fatalf("%+v", got)
	}
}
