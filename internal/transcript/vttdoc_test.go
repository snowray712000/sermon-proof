package transcript

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func vttRoundTrip(t *testing.T, name, in string) *Document {
	t.Helper()
	d, err := ParseVTTDoc(in)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if out := d.Render(); out != in {
		t.Fatalf("%s: round-trip differs (len in=%d out=%d)\nin:  %q\nout: %q", name, len(in), len(out), in, out)
	}
	return d
}

func TestVTTDocRoundTripTestdata(t *testing.T) {
	files, _ := filepath.Glob("../../testdata/*.vtt")
	if len(files) < 2 {
		t.Fatal("no vtt testdata")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		d := vttRoundTrip(t, f, string(b))
		if d.NumSegments() == 0 || len(d.Meta) == 0 {
			t.Fatalf("%s: %d cues, %d meta", f, d.NumSegments(), len(d.Meta))
		}
		for _, s := range d.Segments() {
			if s.ReadOnly || s.EndMs <= s.StartMs {
				t.Fatalf("%s: cue %s readonly=%v end=%d", f, s.Time, s.ReadOnly, s.EndMs)
			}
		}
	}
}

// SERMON_PROOF_CORPUS=/path/to/folder also covers every .vtt there (read only).
func TestVTTDocRoundTripCorpus(t *testing.T) {
	dir := os.Getenv("SERMON_PROOF_CORPUS")
	if dir == "" {
		t.Skip("SERMON_PROOF_CORPUS not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.vtt"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		d := vttRoundTrip(t, f, string(b))
		v, err := ParseVTT(string(b)) // the lossy reader must agree on the cue count
		if err != nil || len(v.Cues) != d.NumSegments() {
			t.Fatalf("%s: ParseVTT cues=%v vs %d (%v)", f, len(v.Cues), d.NumSegments(), err)
		}
	}
	t.Logf("%d files", len(files))
}

func TestVTTDocMatchesLossyParser(t *testing.T) {
	b, _ := os.ReadFile("../../testdata/N59_1_12_1_13-ver1.orig.vtt")
	d := vttRoundTrip(t, "orig", string(b))
	v, err := ParseVTT(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Cues) != d.NumSegments() {
		t.Fatalf("%d vs %d", len(v.Cues), d.NumSegments())
	}
	for i, s := range d.Segments() {
		c := v.Cues[i]
		if s.StartMs != c.StartMs || s.EndMs != c.EndMs || s.Body != c.Text {
			t.Fatalf("cue %d differs: %+v vs %+v", i, s, c)
		}
	}
	if !strings.Contains(d.Render(), "[疑似：") {
		t.Fatal("machine draft lost its suspect markers")
	}
	if got := (KV{"book", "雅各書 (N59)"}); d.Meta[0] != got {
		t.Fatalf("meta[0]=%v", d.Meta[0])
	}
}

func TestVTTDocEdgeCases(t *testing.T) {
	cases := map[string]string{
		"header only":       "WEBVTT\n",
		"header no eol":     "WEBVTT",
		"header title":      "WEBVTT - some title\nKind: captions\n\n00:00.000 --> 00:01.000\na\n",
		"bom":               "\xef\xbb\xbfWEBVTT\n\nNOTE\nbook: x\n\ns001\n00:00.000 --> 00:01.000\nhi\n",
		"crlf":              "WEBVTT\r\n\r\ns1\r\n00:00.000 --> 00:01.000\r\nhi\r\nthere\r\n\r\n",
		"mixed eol":         "WEBVTT\r\n\n00:00.000 --> 00:01.000\r\na\nb\r\n\r\n00:01.000 --> 00:02.000\nc\r\n",
		"no trailing eol":   "WEBVTT\n\n00:00.000 --> 00:01.000\nlast",
		"no id":             "WEBVTT\n\n00:00.000 --> 00:01.000\nx\n",
		"cue settings":      "WEBVTT\n\nid\n00:00.000 --> 00:01.000 align:start position:10%\nx\n",
		"hours":             "WEBVTT\n\n01:02:03.004 --> 100:00:00.000\nx\n",
		"style region note": "WEBVTT\n\nSTYLE\n::cue { color: red }\n\nREGION\nid:r\n\nNOTE a comment\nover two lines\n\n00:00.000 --> 00:01.000\nx\n",
		"empty payload":     "WEBVTT\n\n00:00.000 --> 00:01.000\n\n00:01.000 --> 00:02.000\nx\n",
		"empty at eof":      "WEBVTT\n\n00:00.000 --> 00:01.000",
		"empty at eof nl":   "WEBVTT\n\n00:00.000 --> 00:01.000\n",
		"many blanks":       "WEBVTT\n\n\n\n00:00.000 --> 00:01.000\nx\n\n\n\n",
		"spaces line":       "WEBVTT\n\n00:00.000 --> 00:01.000\nx\n  \ny\n",
		"tags":              "WEBVTT\n\n00:00.000 --> 00:01.000\n<v Bob>hi <b>there</b>\n",
		"bare amp":          "WEBVTT\n\n00:00.000 --> 00:01.000\nAT&T &nbsp;\n",
		"arrow in payload":  "WEBVTT\n\n00:00.000 --> 00:01.000\na --> b\n",
		"bad timing":        "WEBVTT\n\n00:00.0 --> 00:01.000\nx\n",
		"timing no id":      "WEBVTT\n\nx\ny\nz\n",
		"id with arrow":     "WEBVTT\n\na --> b\n00:00.000 --> 00:01.000\nx\n",
		"lone cr":           "WEBVTT\n\n00:00.000 --> 00:01.000\nx\ry\n",
		"escaped":           "WEBVTT\n\n00:00.000 --> 00:01.000\na &amp; b &lt;c&gt; &amp;lt;\n",
		"unicode id":        "WEBVTT\n\n第一段\n00:00.000 --> 00:01.000\n文字\n",
	}
	for n, c := range cases {
		vttRoundTrip(t, n, c)
	}
}

func TestVTTDocRejectsNonVTT(t *testing.T) {
	for _, s := range []string{"", "\n", "# md\n", "WEBVTTX\n", " WEBVTT\n", "**[00:00.000]** a\n"} {
		if _, err := ParseVTTDoc(s); err == nil {
			t.Fatalf("accepted %q", s)
		}
	}
}

func TestVTTDocRoundTripRandom(t *testing.T) {
	pieces := []string{
		"\n", "\r\n", "\n", "\n\n", "00:01.000 --> 00:02.500", "01:02:03.004 --> 01:02:04.000 line:0",
		"s001", "NOTE", "NOTE x", "STYLE", "REGION", "文字", "，", " ", "&amp;", "&", "<b>", "&lt;", "-->",
		"[疑似：詞，00:05.420]", "\r", "\xef\xbb\xbf", "key: value", "id:r",
	}
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 4000; i++ {
		var sb strings.Builder
		switch rng.Intn(3) {
		case 0:
			sb.WriteString("WEBVTT\n\n")
		case 1:
			sb.WriteString("\xef\xbb\xbfWEBVTT\r\n\r\n")
		default:
			sb.WriteString("WEBVTT - t\n")
		}
		for j := rng.Intn(40); j > 0; j-- {
			sb.WriteString(pieces[rng.Intn(len(pieces))])
		}
		vttRoundTrip(t, "random", sb.String())
	}
}

const vttSample = "WEBVTT\r\n\r\nNOTE\r\nbook: 雅各書 (N59)\r\nstatus: proofread\r\n\r\ns001\r\n00:00.000 --> 00:05.000\r\n第一段\r\n\r\ns002\r\n00:05.000 --> 00:09.000 align:start\r\n第二段\r\n第二行\r\n\r\ns003\r\n00:09.000 --> 00:12.000\r\na &amp; b &lt;c&gt;\r\n"

func TestVTTDocParseCues(t *testing.T) {
	d := vttRoundTrip(t, "sample", vttSample)
	if d.Format != FormatVTT || d.EOL != "\r\n" || d.NumSegments() != 3 {
		t.Fatalf("format=%q eol=%q n=%d", d.Format, d.EOL, d.NumSegments())
	}
	if len(d.Meta) != 2 || d.Meta[1] != (KV{"status", "proofread"}) {
		t.Fatalf("meta=%v", d.Meta)
	}
	s := d.Segment(1)
	if s.StartMs != 5000 || s.EndMs != 9000 || s.Time != "00:05.000" || s.Body != "第二段\r\n第二行" {
		t.Fatalf("seg1=%+v", s)
	}
	if got := d.Segment(2).Body; got != "a & b <c>" {
		t.Fatalf("unescape: %q", got)
	}
}

func TestVTTDocEditOnlyChangesThatCue(t *testing.T) {
	d, _ := ParseVTTDoc(vttSample)
	n, err := d.Apply(map[int]string{1: "改過\n兩行 & <x>"})
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	want := strings.Replace(vttSample, "第二段\r\n第二行", "改過\r\n兩行 &amp; &lt;x&gt;", 1)
	if got := d.Render(); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	// and the result reads back to the same text
	d2 := vttRoundTrip(t, "edited", want)
	if d2.Segment(1).Body != "改過\r\n兩行 & <x>" {
		t.Fatalf("read back %q", d2.Segment(1).Body)
	}
	// editing to the identical text is not a change
	if n, _ := d2.Apply(map[int]string{1: "改過\n兩行 & <x>"}); n != 0 {
		t.Fatalf("no-op counted as change: %d", n)
	}
}

func TestVTTDocEditEmptyPayloadCases(t *testing.T) {
	// a cue without payload gains text
	d, _ := ParseVTTDoc("WEBVTT\n\n00:00.000 --> 00:01.000\n\n00:01.000 --> 00:02.000\nx\n")
	d.Apply(map[int]string{0: "新"})
	if got, want := d.Render(), "WEBVTT\n\n00:00.000 --> 00:01.000\n新\n\n00:01.000 --> 00:02.000\nx\n"; got != want {
		t.Fatalf("got %q", got)
	}
	// ... also when the timing line is the unterminated end of the file
	d, _ = ParseVTTDoc("WEBVTT\r\n\r\n00:00.000 --> 00:01.000")
	d.Apply(map[int]string{0: "新"})
	if got, want := d.Render(), "WEBVTT\r\n\r\n00:00.000 --> 00:01.000\r\n新"; got != want {
		t.Fatalf("got %q", got)
	}
	// a cue emptied: no stray blank line, next cue intact
	d, _ = ParseVTTDoc("WEBVTT\n\n00:00.000 --> 00:01.000\nabc\n\n00:01.000 --> 00:02.000\nx\n")
	d.Apply(map[int]string{0: ""})
	if got, want := d.Render(), "WEBVTT\n\n00:00.000 --> 00:01.000\n\n00:01.000 --> 00:02.000\nx\n"; got != want {
		t.Fatalf("got %q", got)
	}
	// last cue without trailing newline keeps that property
	d, _ = ParseVTTDoc("WEBVTT\n\n00:00.000 --> 00:01.000\nlast")
	d.Apply(map[int]string{0: "changed"})
	if got, want := d.Render(), "WEBVTT\n\n00:00.000 --> 00:01.000\nchanged"; got != want {
		t.Fatalf("got %q", got)
	}
}

func TestVTTDocRejectsUnwritableEdits(t *testing.T) {
	d, _ := ParseVTTDoc("WEBVTT\n\n00:00.000 --> 00:01.000\na\n\n00:01.000 --> 00:02.000\n<b>x</b>\n")
	for _, bad := range []string{"a\n\nb", "\nb", "b\n"} {
		if _, err := d.Clone().Apply(map[int]string{0: bad}); err == nil {
			t.Fatalf("accepted blank line in %q", bad)
		}
	}
	if !d.Segment(1).ReadOnly {
		t.Fatal("cue with tags should be read only")
	}
	if _, err := d.Clone().Apply(map[int]string{1: "y"}); err == nil {
		t.Fatal("edited a read-only cue")
	}
	// the document is untouched by the failed attempts
	if got := d.Render(); got != "WEBVTT\n\n00:00.000 --> 00:01.000\na\n\n00:01.000 --> 00:02.000\n<b>x</b>\n" {
		t.Fatalf("render changed: %q", got)
	}
}

func TestVTTDocEditOnTestdata(t *testing.T) {
	b, _ := os.ReadFile("../../testdata/N59_5_10_5_11-ver1.vtt")
	in := string(b)
	d, _ := ParseVTTDoc(in)
	if _, err := d.Apply(map[int]string{2: "只改這一段 & <測試>"}); err != nil {
		t.Fatal(err)
	}
	out := d.Render()
	a, c := strings.Split(in, "\n\n"), strings.Split(out, "\n\n")
	if len(a) != len(c) {
		t.Fatalf("block count %d -> %d", len(a), len(c))
	}
	diff := 0
	for i := range a {
		if a[i] != c[i] {
			diff++
			if !strings.Contains(c[i], "只改這一段 &amp; &lt;測試&gt;") || !strings.HasPrefix(c[i], strings.SplitN(a[i], "\n", 3)[0]+"\n"+strings.SplitN(a[i], "\n", 3)[1]+"\n") {
				t.Fatalf("changed block wrong: %q", c[i])
			}
		}
	}
	if diff != 1 {
		t.Fatalf("%d blocks changed", diff)
	}
}

func TestPairAndNamesForVTT(t *testing.T) {
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "N59_5_10_5_11.mp3")
	os.WriteFile(mp3, []byte("x"), 0o644)
	for _, n := range []string{"N59_5_10_5_11-ver1.vtt", "N59_5_10_5_11.VTT", "N59_5_10_5_11-ver1.md"} {
		if p, ok := PairAudio(filepath.Join(dir, n)); !ok || p != mp3 {
			t.Fatalf("%s -> %q %v", n, p, ok)
		}
	}
	if !IsOrigName("X-ver1.orig.vtt") || !IsOrigName("X-ver1.orig.md") || IsOrigName("X-ver1.vtt") || IsOrigName("X.md") {
		t.Fatal("IsOrigName")
	}
	if OrigPath("a/X-ver1.vtt") != "a/X-ver1.orig.vtt" || OrigPath("a/X-ver1.md") != "a/X-ver1.orig.md" {
		t.Fatal("OrigPath")
	}
	if ReviewPath("a/X-ver1.vtt") == ReviewPath("a/X-ver1.md") {
		t.Fatal("md and vtt share a review sidecar")
	}
	if ReviewPath("a/X-ver1.md") != "a/X-ver1.review.json" {
		t.Fatal("md sidecar name changed")
	}
}
