// Package audio serves audio so that browser seeking matches real time.
//
// Browsers estimate the duration of an MP3 that lacks a Xing/Info header from
// file size / nominal bitrate. For the pipeline's files (frames of 156 bytes
// with no padding) that is ~0.47% short, so currentTime drifts from the true
// position by ~5 s at 17 minutes and every transcript timestamp lands in the
// wrong place. We therefore serve such files with a synthesized Xing frame
// (exact frame count + a real TOC) in front of the audio frames. The file on
// disk is never touched.
package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

var (
	br1 = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	br2 = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
	sr1 = [3]int{44100, 48000, 32000}
)

type frameInfo struct {
	length  int
	samples int
	mpeg1   bool
	mono    bool
	hdr     [4]byte
	brIdx   int
}

// parseFrame decodes an MPEG audio Layer III frame header at b[0:4].
func parseFrame(b []byte) (frameInfo, bool) {
	var f frameInfo
	if len(b) < 4 || b[0] != 0xFF || b[1]&0xE0 != 0xE0 {
		return f, false
	}
	ver := (b[1] >> 3) & 3 // 3 = MPEG1, 2 = MPEG2, 0 = MPEG2.5
	layer := (b[1] >> 1) & 3
	if ver == 1 || layer != 1 { // reserved version, or not Layer III
		return f, false
	}
	bi, si := int(b[2]>>4), int((b[2]>>2)&3)
	if bi == 0 || bi == 15 || si == 3 {
		return f, false
	}
	f.mpeg1 = ver == 3
	var bitrate, sr int
	if f.mpeg1 {
		bitrate, sr, f.samples = br1[bi], sr1[si], 1152
	} else {
		bitrate, sr, f.samples = br2[bi], sr1[si]/2, 576
		if ver == 0 {
			sr = sr1[si] / 4
		}
	}
	pad := int((b[2] >> 1) & 1)
	f.length = f.samples/8*bitrate*1000/sr + pad
	f.mono = b[3]>>6 == 3
	f.brIdx = bi
	copy(f.hdr[:], b[:4])
	return f, f.length >= 4
}

func (f frameInfo) sideInfo() int {
	switch {
	case f.mpeg1 && f.mono:
		return 17
	case f.mpeg1:
		return 32
	case f.mono:
		return 9
	}
	return 17
}

// id3v2Len returns the size of a leading ID3v2 tag (0 if none).
func id3v2Len(b []byte) int {
	if len(b) < 10 || string(b[:3]) != "ID3" {
		return 0
	}
	n := int(b[6]&0x7F)<<21 | int(b[7]&0x7F)<<14 | int(b[8]&0x7F)<<7 | int(b[9]&0x7F)
	n += 10
	if b[5]&0x10 != 0 { // footer present
		n += 10
	}
	return n
}

// Plan describes how to serve one file: head bytes followed by file[off:off+n].
type Plan struct {
	Head  []byte
	Off   int64
	N     int64
	Fixed bool // false: serve the file as is
	size  int64
	mtime time.Time
}

// Size is the length of the served stream.
func (p *Plan) Size() int64 {
	if !p.Fixed {
		return p.size
	}
	return int64(len(p.Head)) + p.N
}

// ModTime of the underlying file.
func (p *Plan) ModTime() time.Time { return p.mtime }

// BuildPlan inspects an audio file. Anything that is not a plain MP3 without
// a Xing/Info/VBRI header gets a pass-through plan.
func BuildPlan(path string) (*Plan, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	plain := &Plan{size: st.Size(), mtime: st.ModTime()}
	if st.Size() > 512<<20 {
		return plain, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	start := id3v2Len(data)
	if start >= len(data) {
		return plain, nil
	}
	// Allow a little garbage before the first frame.
	first := -1
	for i := start; i < len(data)-4 && i < start+4096; i++ {
		if f, ok := parseFrame(data[i:]); ok && i+f.length+4 <= len(data) {
			if _, ok2 := parseFrame(data[i+f.length:]); ok2 {
				first = i
				break
			}
		}
	}
	if first < 0 {
		return plain, nil
	}
	f0, _ := parseFrame(data[first:])
	tagOff := first + 4 + f0.sideInfo()
	if tagOff+4 <= len(data) {
		switch string(data[tagOff : tagOff+4]) {
		case "Xing", "Info":
			return plain, nil
		}
	}
	if first+36 <= len(data) && string(data[first+36:first+40]) == "VBRI" {
		return plain, nil
	}

	var offsets []int
	cbr := true
	pos := first
	for pos+4 <= len(data) {
		f, ok := parseFrame(data[pos:])
		if !ok || pos+f.length > len(data) {
			break
		}
		if f.brIdx != f0.brIdx {
			cbr = false
		}
		offsets = append(offsets, pos)
		pos += f.length
	}
	end := pos
	nFrames := len(offsets)
	if nFrames < 100 {
		return plain, nil
	}
	audioBytes := end - first

	head, err := xingFrame(f0, cbr, nFrames, audioBytes, offsets, first)
	if err != nil {
		return plain, nil
	}
	return &Plan{Head: head, Off: int64(first), N: int64(audioBytes), Fixed: true, size: st.Size(), mtime: st.ModTime()}, nil
}

// xingFrame builds the tag frame with f0's stream parameters.
//
// CBR streams get an "Info" tag with frames + bytes only: decoders then seek
// linearly, which is exact. A TOC would hurt there, because its entries are
// quantised to 1/256 of the file (~4 s for a 17 minute talk). VBR streams get
// a "Xing" tag with a real TOC.
func xingFrame(f0 frameInfo, cbr bool, nFrames, audioBytes int, offsets []int, first int) ([]byte, error) {
	need := 4 + f0.sideInfo() + 4 + 4 + 4 + 4
	if !cbr {
		need += 100
	}
	f := f0
	// A tiny first frame cannot hold the tag: raise the (irrelevant) bitrate.
	for f.samples/8*bitrateOf(f, f.brIdx)*1000/sampleRateOf(f) < need {
		f.brIdx++
		if f.brIdx > 14 {
			return nil, errors.New("cannot fit Xing frame")
		}
	}
	length := f.samples / 8 * bitrateOf(f, f.brIdx) * 1000 / sampleRateOf(f)
	hdr := f.hdr
	hdr[1] |= 1                             // no CRC
	hdr[2] = hdr[2]&0x0F | byte(f.brIdx)<<4 // chosen bitrate index
	hdr[2] &^= 0x02                         // no padding
	buf := make([]byte, length)
	copy(buf, hdr[:])
	o := 4 + f0.sideInfo()
	tag, flags := "Xing", uint32(0x07) // frames | bytes | toc
	if cbr {
		tag, flags = "Info", 0x03 // frames | bytes
	}
	copy(buf[o:], tag)
	binary.BigEndian.PutUint32(buf[o+4:], flags)
	binary.BigEndian.PutUint32(buf[o+8:], uint32(nFrames))
	binary.BigEndian.PutUint32(buf[o+12:], uint32(audioBytes+length))
	for i := 0; i < 100 && !cbr; i++ {
		fr := offsets[i*nFrames/100]
		v := 256 * (fr - first) / audioBytes
		if v > 255 {
			v = 255
		}
		buf[o+16+i] = byte(v)
	}
	return buf, nil
}

func bitrateOf(f frameInfo, idx int) int {
	if f.mpeg1 {
		return br1[idx]
	}
	return br2[idx]
}

// sampleRateOf recovers the sample rate from the saved header.
func sampleRateOf(f frameInfo) int {
	si := int((f.hdr[2] >> 2) & 3)
	ver := (f.hdr[1] >> 3) & 3
	switch ver {
	case 3:
		return sr1[si]
	case 2:
		return sr1[si] / 2
	}
	return sr1[si] / 4
}

// Source is a seekable view of a Plan.
type Source struct {
	plan *Plan
	f    *os.File
	pos  int64
}

// Open returns a Source for the plan. The caller must Close it.
func (p *Plan) Open(path string) (*Source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &Source{plan: p, f: f}, nil
}

func (s *Source) Close() error { return s.f.Close() }

func (s *Source) Read(b []byte) (int, error) {
	if !s.plan.Fixed {
		return s.f.Read(b)
	}
	total := s.plan.Size()
	if s.pos >= total {
		return 0, io.EOF
	}
	hl := int64(len(s.plan.Head))
	var n int
	if s.pos < hl {
		n = copy(b, s.plan.Head[s.pos:])
	} else {
		rem := total - s.pos
		if int64(len(b)) > rem {
			b = b[:rem]
		}
		var err error
		n, err = s.f.ReadAt(b, s.plan.Off+(s.pos-hl))
		if err != nil && !(errors.Is(err, io.EOF) && n > 0) {
			s.pos += int64(n)
			return n, err
		}
	}
	s.pos += int64(n)
	return n, nil
}

func (s *Source) Seek(off int64, whence int) (int64, error) {
	if !s.plan.Fixed {
		return s.f.Seek(off, whence)
	}
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		off += s.pos
	case io.SeekEnd:
		off += s.plan.Size()
	}
	if off < 0 {
		return 0, errors.New("negative seek")
	}
	s.pos = off
	return off, nil
}

// Cache memoises plans per file (path + size + mtime).
type Cache struct {
	mu sync.Mutex
	m  map[string]*Plan
}

// Get returns a cached or freshly built plan.
func (c *Cache) Get(path string) (*Plan, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	key := fmt.Sprintf("%s|%d|%d", path, st.ModTime().UnixNano(), st.Size())
	c.mu.Lock()
	defer c.mu.Unlock()
	if p, ok := c.m[key]; ok {
		return p, nil
	}
	p, err := BuildPlan(path)
	if err != nil {
		return nil, err
	}
	c.m = map[string]*Plan{key: p} // keep only the latest file
	return p, nil
}
