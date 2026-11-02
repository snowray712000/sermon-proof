package server

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sermonproof/internal/transcript"
)

func TestSessionSaveFlow(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile("../../testdata/N59_1_12_1_13-ver1.md")
	if err != nil {
		t.Fatal(err)
	}
	md := filepath.Join(dir, "N59_1_12_1_13-ver1.md")
	os.WriteFile(md, src, 0o644)
	os.WriteFile(filepath.Join(dir, "N59_1_12_1_13.mp3"), []byte("x"), 0o644)

	s := &Session{}
	if err := s.Open(md); err != nil {
		t.Fatal(err)
	}
	st := s.State()
	if !st.Open || len(st.Segments) != 98 || !st.HasAudio || st.EOL != "crlf" || st.HasOrig {
		t.Fatalf("state: open=%v segs=%d audio=%v eol=%s orig=%v", st.Open, len(st.Segments), st.HasAudio, st.EOL, st.HasOrig)
	}

	// no-op save writes nothing and makes no backup
	if _, err := s.Save(st.Hash, []Edit{{ID: 0, Text: st.Segments[0].Body}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(transcript.OrigPath(md)); err == nil {
		t.Fatal("backup made for a no-op save")
	}

	st2, err := s.Save(st.Hash, []Edit{{ID: 2, Text: "改過的一段"}})
	if err != nil {
		t.Fatal(err)
	}
	if !st2.HasOrig || st2.Segments[2].Body != "改過的一段" || st2.Hash == st.Hash {
		t.Fatalf("after save: %+v", st2.HasOrig)
	}
	got, _ := os.ReadFile(md)
	orig, _ := os.ReadFile(transcript.OrigPath(md))
	if string(orig) != string(src) {
		t.Fatal("backup differs from original")
	}
	if strings.Count(string(got), "\n") != strings.Count(string(src), "\n") || !strings.Contains(string(got), "改過的一段\r\n") {
		t.Fatal("saved file lost its CRLF structure")
	}

	// stale baseHash is a conflict
	if _, err := s.Save(st.Hash, []Edit{{ID: 3, Text: "x"}}); !errors.Is(err, transcript.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	// backup files are refused
	if err := s.Open(transcript.OrigPath(md)); err == nil {
		t.Fatal("opened .orig.md")
	}
}

func TestSessionVTTSaveFlow(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile("../../testdata/N59_5_10_5_11-ver1.vtt")
	if err != nil {
		t.Fatal(err)
	}
	vtt := filepath.Join(dir, "N59_5_10_5_11-ver1.vtt")
	os.WriteFile(vtt, src, 0o644)
	os.WriteFile(filepath.Join(dir, "N59_5_10_5_11.mp3"), []byte("x"), 0o644)

	s := &Session{}
	if err := s.Open(vtt); err != nil {
		t.Fatal(err)
	}
	st := s.State()
	if !st.Open || st.Format != "vtt" || !st.HasAudio || st.HasOrig || len(st.Segments) == 0 {
		t.Fatalf("state: %+v", st)
	}
	if len(st.Meta) == 0 || st.Meta[0].Key != "book" || st.Meta[0].Value != "雅各書 (N59)" {
		t.Fatalf("meta: %v", st.Meta)
	}
	if st.Segments[0].EndMs != st.Segments[1].StartMs || st.Segments[0].Time != "00:00.000" {
		t.Fatalf("segment 0: %+v", st.Segments[0])
	}

	// no-op save: nothing written, no backup
	if _, err := s.Save(st.Hash, []Edit{{ID: 0, Text: st.Segments[0].Body}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(transcript.OrigPath(vtt)); err == nil {
		t.Fatal("backup made for a no-op save")
	}

	st2, err := s.Save(st.Hash, []Edit{{ID: 2, Text: "改過 & <一段>"}})
	if err != nil {
		t.Fatal(err)
	}
	if !st2.HasOrig || st2.Segments[2].Body != "改過 & <一段>" || st2.Hash == st.Hash {
		t.Fatalf("after save: orig=%v body=%q", st2.HasOrig, st2.Segments[2].Body)
	}
	got, _ := os.ReadFile(vtt)
	orig, _ := os.ReadFile(transcript.OrigPath(vtt))
	if filepath.Base(transcript.OrigPath(vtt)) != "N59_5_10_5_11-ver1.orig.vtt" || string(orig) != string(src) {
		t.Fatal("backup missing or differs from original")
	}
	if !strings.Contains(string(got), "\n改過 &amp; &lt;一段&gt;\n") || transcript.Hash(got) != st2.Hash {
		t.Fatal("saved file wrong")
	}
	if strings.Count(string(got), "\n") != strings.Count(string(src), "\n") {
		t.Fatal("saved file changed its line structure")
	}

	// a second save leaves the first backup alone
	if _, err := s.Save(st2.Hash, []Edit{{ID: 3, Text: "又改"}}); err != nil {
		t.Fatal(err)
	}
	if orig2, _ := os.ReadFile(transcript.OrigPath(vtt)); string(orig2) != string(src) {
		t.Fatal("backup was overwritten")
	}

	// hash conflicts: stale baseHash, and the file changed behind our back
	if _, err := s.Save(st.Hash, []Edit{{ID: 4, Text: "x"}}); !errors.Is(err, transcript.ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	cur := s.State()
	os.WriteFile(vtt, append(got, '\n'), 0o644)
	if _, err := s.Save(cur.Hash, []Edit{{ID: 4, Text: "x"}}); !errors.Is(err, transcript.ErrConflict) {
		t.Fatalf("external change not detected: %v", err)
	}

	// bad edits are refused and leave the file alone
	os.WriteFile(vtt, src, 0o644)
	s.Reload()
	before, _ := os.ReadFile(vtt)
	if _, err := s.Save(s.State().Hash, []Edit{{ID: 1, Text: "a\n\nb"}}); err == nil {
		t.Fatal("blank line in a cue accepted")
	}
	if after, _ := os.ReadFile(vtt); string(after) != string(before) {
		t.Fatal("file changed by a refused save")
	}

	// sidecars: .vtt and .md of the same name do not collide
	if err := s.SaveReview(&transcript.Review{Segments: map[string]bool{"0": true}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "N59_5_10_5_11-ver1.vtt.review.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "N59_5_10_5_11-ver1.review.json")); err == nil {
		t.Fatal("vtt wrote the md sidecar")
	}

	// backups and other extensions are refused
	if err := s.Open(transcript.OrigPath(vtt)); err == nil {
		t.Fatal("opened .orig.vtt")
	}
	txt := filepath.Join(dir, "a.txt")
	os.WriteFile(txt, []byte("WEBVTT\n"), 0o644)
	if err := s.Open(txt); err == nil {
		t.Fatal("opened .txt")
	}
	bad := filepath.Join(dir, "bad.vtt")
	os.WriteFile(bad, []byte("not vtt"), 0o644)
	if err := s.Open(bad); err == nil {
		t.Fatal("opened a .vtt without WEBVTT header")
	}
	if s.State().MdName != "N59_5_10_5_11-ver1.vtt" {
		t.Fatal("a failed open replaced the session")
	}
}

func TestSessionMDStateUnchanged(t *testing.T) {
	dir := t.TempDir()
	src, _ := os.ReadFile("../../testdata/N59_1_12_1_13-ver1.md")
	md := filepath.Join(dir, "N59_1_12_1_13-ver1.md")
	os.WriteFile(md, src, 0o644)
	s := &Session{}
	if err := s.Open(md); err != nil {
		t.Fatal(err)
	}
	st := s.State()
	if st.Format != "md" || len(st.Meta) != 0 || st.Segments[0].EndMs != 0 {
		t.Fatalf("md state: format=%q meta=%v end=%d", st.Format, st.Meta, st.Segments[0].EndMs)
	}
}

func TestDropKind(t *testing.T) {
	for n, want := range map[string]string{"a.md": "md", "a.VTT": "md", "a.mp3": "audio", "a.txt": "", "a.orig.vtt": "md"} {
		if got := dropKind(n); got != want {
			t.Fatalf("%s: %q", n, got)
		}
	}
}

func TestSessionSiblings(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{
		"A_1-ver1.md", "A_1.mp3", "A_2-ver1.vtt", "A_2-ver1.orig.vtt", "A_10-ver1.md",
		"._A_1-ver1.md", "notes.txt", "A_1-ver1.md.review.json",
	} {
		os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644)
	}
	os.Mkdir(filepath.Join(dir, "sub.md"), 0o755)
	src, _ := os.ReadFile("../../testdata/N59_1_12_1_13-ver1.md")
	os.WriteFile(filepath.Join(dir, "A_1-ver1.md"), src, 0o644)
	os.WriteFile(filepath.Join(dir, "A_10-ver1.md"), src, 0o644)

	s := &Session{}
	if l, err := s.Siblings(); err != nil || len(l) != 0 {
		t.Fatalf("nothing open: %v %v", l, err)
	}
	if err := s.OpenSibling("A_10-ver1.md"); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("err=%v", err)
	}
	if err := s.Open(filepath.Join(dir, "A_1-ver1.md")); err != nil {
		t.Fatal(err)
	}
	l, err := s.Siblings()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Sibling{}
	for _, e := range l {
		got[e.Name] = e
	}
	if len(got) != 3 || !got["A_1-ver1.md"].Current || !got["A_1-ver1.md"].HasAudio ||
		got["A_10-ver1.md"].Current || got["A_10-ver1.md"].HasAudio || !got["A_2-ver1.vtt"].HasOrig {
		t.Fatalf("siblings: %+v", l)
	}

	for _, bad := range []string{"../A_1-ver1.md", "A_2-ver1.orig.vtt", "notes.txt", "._A_1-ver1.md", "sub/A.md"} {
		if err := s.OpenSibling(bad); err == nil {
			t.Fatalf("opened %q", bad)
		}
	}
	if err := s.OpenSibling("A_10-ver1.md"); err != nil {
		t.Fatal(err)
	}
	if st := s.State(); st.MdName != "A_10-ver1.md" || st.HasAudio {
		t.Fatalf("after switch: %s audio=%v", st.MdName, st.HasAudio)
	}
}

func TestSessionExportVTT(t *testing.T) {
	dir := t.TempDir()
	src, _ := os.ReadFile("../../testdata/N59_1_12_1_13-ver1.md")
	md := filepath.Join(dir, "N59_1_12_1_13-ver1.md")
	os.WriteFile(md, src, 0o644)
	s := &Session{}
	if _, err := s.ExportVTT("proofread", false); !errors.Is(err, ErrNotOpen) {
		t.Fatalf("err=%v", err)
	}
	if err := s.Open(md); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExportVTT("  ", false); err == nil {
		t.Fatal("empty status accepted")
	}
	r, err := s.ExportVTT("proofread", false)
	if err != nil || r.Name != "N59_1_12_1_13-ver1.vtt" || r.Cues != 98 || r.Replaced || r.Unchanged {
		t.Fatalf("%+v %v", r, err)
	}
	vtt := filepath.Join(dir, r.Name)
	first, _ := os.ReadFile(vtt)
	if got, _ := os.ReadFile(md); string(got) != string(src) {
		t.Fatal("export touched the md")
	}

	// same content again: nothing to do, even without overwrite
	if r, err = s.ExportVTT("proofread", false); err != nil || !r.Unchanged {
		t.Fatalf("%+v %v", r, err)
	}
	// a different declaration differs from the file: refuse without overwrite, replace with it
	if _, err = s.ExportVTT("in-progress", false); !errors.Is(err, ErrVTTExists) {
		t.Fatalf("err=%v", err)
	}
	if got, _ := os.ReadFile(vtt); string(got) != string(first) {
		t.Fatal("refused export changed the file")
	}
	if r, err = s.ExportVTT("in-progress", true); err != nil || !r.Replaced {
		t.Fatalf("%+v %v", r, err)
	}
	if got, _ := os.ReadFile(vtt); !strings.Contains(string(got), "status: in-progress") {
		t.Fatal("not replaced")
	}

	// after a save the export follows the new text; an out-of-date session is refused
	st := s.State()
	if _, err := s.Save(st.Hash, []Edit{{ID: 1, Text: "改過"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExportVTT("in-progress", true); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(vtt); !strings.Contains(string(got), "\n改過\n") {
		t.Fatal("export is not of the saved text")
	}
	os.WriteFile(md, append(src, '\n'), 0o644)
	if _, err := s.ExportVTT("in-progress", true); !errors.Is(err, transcript.ErrConflict) {
		t.Fatalf("err=%v", err)
	}

	// a .vtt session cannot export
	s2 := &Session{}
	s2.Open(vtt)
	if _, err := s2.ExportVTT("x", true); err == nil {
		t.Fatal("exported from a vtt")
	}
}
