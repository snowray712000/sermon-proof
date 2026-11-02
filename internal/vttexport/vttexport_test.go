package vttexport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sermonproof/internal/transcript"
)

func TestFromMDNamesStatusAndMeta(t *testing.T) {
	src, err := os.ReadFile("../../testdata/N59_1_12_1_13-ver1.md")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "康來昌", "N59_雅各書")
	os.MkdirAll(dir, 0o755)

	r, err := FromMD(filepath.Join(dir, "N59_1_12_1_13-ver1.md"), src, Options{Status: "proofread"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "N59_1_12_1_13-ver1.vtt" || r.Cues != 98 || r.Suspects == 0 {
		t.Fatalf("%+v", r.Name)
	}
	v, err := transcript.ParseVTT(r.Text)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, kv := range v.Meta {
		got[kv.Key] = kv.Value
	}
	if got["book"] != "雅各書 (N59)" || got["passage"] != "1:12–13" || got["speaker"] != "康來昌" ||
		got["status"] != "proofread" || got["draft"] != "N59_1_12_1_13-ver1.orig.vtt" || got["audio"] != "" {
		t.Fatalf("meta %v", got)
	}

	// the machine draft never takes the declared status and has its own file name
	o, err := FromMD(filepath.Join(dir, "N59_1_12_1_13-ver1.orig.md"), src, Options{Status: "proofread"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Name != "N59_1_12_1_13-ver1.orig.vtt" || !strings.Contains(o.Text, "status: machine-draft") || strings.Contains(o.Text, "status: proofread") {
		t.Fatal("orig export wrong")
	}

	// a free-form name still converts, with partial background
	f, err := FromMD(filepath.Join(dir, "講道.md"), src, Options{Status: "draft"})
	if err != nil || f.Name != "講道.vtt" || strings.Contains(f.Text, "book:") || !strings.Contains(f.Text, "status: draft") {
		t.Fatalf("free-form: %v", err)
	}

	// the result is a valid file the editor reads back byte-exact
	d, err := transcript.ParseVTTDoc(r.Text)
	if err != nil || d.NumSegments() != 98 || d.Render() != r.Text {
		t.Fatalf("not round-trip: %v", err)
	}
}

func TestFromMDRejects(t *testing.T) {
	if _, err := FromMD("a.txt", []byte("x"), Options{}); err == nil {
		t.Fatal("accepted .txt")
	}
	if _, err := FromMD("a.md", []byte("# nothing\n"), Options{}); err == nil {
		t.Fatal("accepted a file without segments")
	}
}
