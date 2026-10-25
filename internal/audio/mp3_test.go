package audio

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// synthMP3 builds n MPEG1 Layer III mono 48 kbps 44.1 kHz frames, all 156
// bytes (no padding), like the pipeline's files.
func synthMP3(n int, id3 bool, xing bool) []byte {
	var b bytes.Buffer
	if id3 {
		b.Write([]byte{'I', 'D', '3', 3, 0, 0, 0, 0, 0, 10})
		b.Write(make([]byte, 10))
	}
	for i := 0; i < n; i++ {
		fr := make([]byte, 156)
		copy(fr, []byte{0xFF, 0xFB, 0x30, 0xC4}) // MPEG1 L3, no CRC, 48k, 44.1k, no pad, mono
		fr[10] = byte(i)
		if xing && i == 0 {
			copy(fr[21:], "Info")
		}
		b.Write(fr)
	}
	return b.Bytes()
}

func writeTmp(t *testing.T, data []byte) string {
	p := filepath.Join(t.TempDir(), "a.mp3")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFixedStreamHasXingAndSameAudio(t *testing.T) {
	src := synthMP3(1000, true, false)
	p := writeTmp(t, src)
	plan, err := BuildPlan(p)
	if err != nil || !plan.Fixed {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	s, _ := plan.Open(p)
	defer s.Close()
	out, err := io.ReadAll(s)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(out)) != plan.Size() {
		t.Fatalf("len %d size %d", len(out), plan.Size())
	}
	hl := len(plan.Head)
	if string(out[21:25]) != "Info" {
		t.Fatalf("no Xing at 21: %q", out[:40])
	}
	if got := binary.BigEndian.Uint32(out[21+8:]); got != 1000 {
		t.Fatalf("frames=%d", got)
	}
	if got := binary.BigEndian.Uint32(out[21+12:]); got != uint32(1000*156+hl) {
		t.Fatalf("bytes=%d", got)
	}
	if !bytes.Equal(out[hl:], src[20:]) { // ID3 header(10)+10 body stripped
		t.Fatal("audio bytes differ")
	}
	if flags := binary.BigEndian.Uint32(out[21+4:]); flags != 3 {
		t.Fatalf("CBR flags=%d, want frames|bytes only", flags)
	}
	// parseable as a frame of its own
	if f, ok := parseFrame(out); !ok || f.length != hl {
		t.Fatalf("head frame len %d vs %d ok=%v", f.length, hl, ok)
	}
}

func TestSeekAndRange(t *testing.T) {
	src := synthMP3(500, false, false)
	p := writeTmp(t, src)
	plan, _ := BuildPlan(p)
	s, _ := plan.Open(p)
	defer s.Close()
	all, _ := io.ReadAll(s)
	for _, c := range []struct{ off, n int }{{0, 10}, {len(plan.Head) - 3, 10}, {len(all) - 5, 5}, {1234, 4000}} {
		if _, err := s.Seek(int64(c.off), io.SeekStart); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, c.n)
		n, _ := io.ReadFull(s, buf)
		if !bytes.Equal(buf[:n], all[c.off:c.off+n]) {
			t.Fatalf("mismatch at %d", c.off)
		}
	}
	if end, _ := s.Seek(0, io.SeekEnd); end != plan.Size() {
		t.Fatalf("end %d", end)
	}
}

func TestPassThrough(t *testing.T) {
	for name, data := range map[string][]byte{
		"has info":  synthMP3(500, false, true),
		"too short": synthMP3(10, false, false),
		"not mp3":   bytes.Repeat([]byte("hello"), 1000),
	} {
		plan, err := BuildPlan(writeTmp(t, data))
		if err != nil || plan.Fixed || plan.Size() != int64(len(data)) {
			t.Errorf("%s: fixed=%v size=%d", name, plan.Fixed, plan.Size())
		}
	}
}

// SERMON_PROOF_MP3 points at a real file; its duration from the frame count
// must be the true one.
func TestRealFile(t *testing.T) {
	p := os.Getenv("SERMON_PROOF_MP3")
	if p == "" {
		t.Skip("SERMON_PROOF_MP3 not set")
	}
	plan, err := BuildPlan(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("fixed=%v head=%d off=%d n=%d size=%d", plan.Fixed, len(plan.Head), plan.Off, plan.N, plan.Size())
	if plan.Fixed {
		frames := binary.BigEndian.Uint32(plan.Head[21+8:])
		t.Logf("frames=%d duration=%.3fs", frames, float64(frames)*1152/44100)
	}
}

func TestVBRGetsXingWithTOC(t *testing.T) {
	var b bytes.Buffer
	for i := 0; i < 400; i++ {
		idx := byte(3 + i%3) // vary the bitrate index -> not CBR
		fr := make([]byte, 400)
		copy(fr, []byte{0xFF, 0xFB, idx<<4 | 0x00, 0xC4})
		// frame length for this idx at 44.1 kHz mono MPEG1
		n := 144 * br1[idx] * 1000 / 44100
		b.Write(fr[:n])
	}
	p := writeTmp(t, b.Bytes())
	plan, err := BuildPlan(p)
	if err != nil || !plan.Fixed {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if string(plan.Head[21:25]) != "Xing" || binary.BigEndian.Uint32(plan.Head[25:]) != 7 {
		t.Fatalf("head=%q", plan.Head[:30])
	}
	toc := plan.Head[21+16 : 21+116]
	for i := 1; i < 100; i++ {
		if toc[i] < toc[i-1] {
			t.Fatalf("toc not monotonic at %d", i)
		}
	}
}
