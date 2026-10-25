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
	if err := s.OpenMD(md); err != nil {
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
	if err := s.OpenMD(transcript.OrigPath(md)); err == nil {
		t.Fatal("opened .orig.md")
	}
}
