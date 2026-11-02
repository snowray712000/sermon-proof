package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVTTFromTestdata(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/*.md")
	if len(files) == 0 {
		t.Fatal("no testdata")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		d := Parse(string(b))
		meta := []KV{{"book", "雅各書 (N59)"}, {"status", "proofread"}}
		v := FromDocument(d, meta, 0)
		out := v.String()
		back, err := ParseVTT(out)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		segs := d.Segments()
		if len(back.Cues) != len(segs) {
			t.Fatalf("%s: %d cues for %d segments", f, len(back.Cues), len(segs))
		}
		for i, s := range segs {
			c := back.Cues[i]
			if c.StartMs != s.StartMs || c.Text != strings.TrimSpace(NormalizeEOL(s.Body, "\n")) {
				t.Fatalf("%s: cue %d differs: %q vs %q", f, i, c.Text, s.Body)
			}
			if c.EndMs <= c.StartMs {
				t.Fatalf("%s: cue %d has end <= start", f, i)
			}
			if i+1 < len(segs) && c.EndMs != segs[i+1].StartMs {
				t.Fatalf("%s: cue %d end %d != next start %d", f, i, c.EndMs, segs[i+1].StartMs)
			}
		}
		if len(back.Meta) != 2 || back.Meta[0] != meta[0] {
			t.Fatalf("%s: meta lost: %v", f, back.Meta)
		}
		if again := back.String(); again != out {
			t.Fatalf("%s: VTT text not stable after parse+render", f)
		}
	}
}

func TestVTTEscapeAndMultiline(t *testing.T) {
	d := Parse("[00:01.000] a <b> & c --> d\r\n  續行\r\n\r\n[00:05.500] 第二段\r\n")
	v := FromDocument(d, nil, 9000)
	out := v.String()
	if strings.Count(out, "-->") != 2 {
		t.Fatalf("payload '-->' not escaped:\n%s", out)
	}
	back, err := ParseVTT(out)
	if err != nil {
		t.Fatal(err)
	}
	if back.Cues[0].Text != "a <b> & c --> d\n  續行" {
		t.Fatalf("got %q", back.Cues[0].Text)
	}
	if back.Cues[1].EndMs != 9000 || back.Cues[0].EndMs != 5500 {
		t.Fatalf("ends: %+v", back.Cues)
	}
}

func TestFormatVTTTime(t *testing.T) {
	if got := FormatVTTTime(61*60000 + 2500); got != "01:01:02.500" {
		t.Fatal(got)
	}
	if got := FormatVTTTime(9680); got != "00:09.680" {
		t.Fatal(got)
	}
}

func TestParseVTTRejectsNonVTT(t *testing.T) {
	if _, err := ParseVTT("# not vtt"); err == nil {
		t.Fatal("want error")
	}
}
