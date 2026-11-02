// Package vttexport turns a .md transcript (the source of truth) into its WebVTT
// copy: one cue per segment, background (book, passage, speaker, audio, status)
// in a leading NOTE block. It reads text only; writing is the caller's job.
package vttexport

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"sermonproof/internal/audio"
	"sermonproof/internal/transcript"
)

var (
	// NameRe matches the sermon naming scheme N59_5_10_5_11-ver1[.orig].md.
	NameRe   = regexp.MustCompile(`^(N\d+)_(\d+)_(\d+)_(\d+)_(\d+)-ver(\d+)(\.orig)?\.md$`)
	folderRe = regexp.MustCompile(`^(N\d+)_(.+)$`)
)

// Options of a conversion.
type Options struct {
	// Status written for a final transcript. Declared by the user, never
	// inferred; a ".orig.md" always gets "machine-draft".
	Status string
}

// Result is a converted transcript, not yet written anywhere.
type Result struct {
	Text       string // the .vtt file content
	Name       string // base name of the .vtt (X-ver1.md -> X-ver1.vtt, X-ver1.orig.md -> X-ver1.orig.vtt)
	Cues       int
	DurationMs int // 0 when no mp3 was found next to the source
	Suspects   int // "[疑似" still present in the source text
}

// FromMD converts the md bytes in data, which were read from path (path is
// used for its name, folder and the mp3 next to it). The result is parsed back
// and compared with the source segments before it is returned.
func FromMD(path string, data []byte, opts Options) (*Result, error) {
	name := filepath.Base(path)
	if !strings.EqualFold(filepath.Ext(name), ".md") {
		return nil, fmt.Errorf("不是 .md 檔")
	}
	stem := name[:len(name)-3]
	isOrig := strings.HasSuffix(stem, ".orig")
	stem = strings.TrimSuffix(stem, ".orig")
	base := strings.SplitN(stem, "-ver", 2)[0]
	m := NameRe.FindStringSubmatch(name) // nil: a free-form name, background is then partial

	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	d := transcript.Parse(text)
	if d.NumSegments() == 0 {
		return nil, fmt.Errorf("沒有任何段落")
	}

	dir := filepath.Dir(path)
	mp3 := base + ".mp3"
	durMs := 0
	if dur, err := audio.Duration(filepath.Join(dir, mp3)); err == nil {
		durMs = int(dur.Milliseconds())
	} else {
		mp3 = ""
	}

	var meta []transcript.KV
	add := func(k, v string) {
		if v != "" {
			meta = append(meta, transcript.KV{Key: k, Value: v})
		}
	}
	if m != nil {
		code, c1, v1, c2, v2 := m[1], m[2], m[3], m[4], m[5]
		abs, _ := filepath.Abs(dir)
		book, speaker := "", ""
		if fm := folderRe.FindStringSubmatch(filepath.Base(abs)); fm != nil && fm[1] == code {
			book = fm[2]
			speaker = filepath.Base(filepath.Dir(abs))
		}
		passage := c1 + ":" + v1
		switch {
		case c1 == c2 && v1 == v2:
		case c1 == c2:
			passage += "–" + v2
		default:
			passage += "–" + c2 + ":" + v2
		}
		if book != "" {
			add("book", book+" ("+code+")")
		} else {
			add("book", code)
		}
		add("passage", passage)
		add("speaker", speaker)
	}
	add("audio", mp3)
	if durMs > 0 {
		add("duration", transcript.FormatVTTTime(durMs))
	}
	if m != nil {
		add("version", "ver"+m[6])
	}
	if isOrig {
		add("status", "machine-draft")
	} else {
		add("status", opts.Status)
		add("draft", stem+".orig.vtt")
	}
	add("timing", "每段開始時間為自動對齊，未經人工校準；結束時間為下一段開始時間")
	note := "「……」與「...」表示停頓、轉折或未完成句，不是刪節"
	if strings.Contains(text, "???") {
		note += "；「???」表示未知"
	}
	add("notation", note)
	add("converted-from", name)

	out := transcript.FromDocument(d, meta, durMs).String()

	back, err := transcript.ParseVTT(out)
	if err != nil {
		return nil, err
	}
	segs := d.Segments()
	if len(back.Cues) != len(segs) {
		return nil, fmt.Errorf("verify: %d cues for %d segments", len(back.Cues), len(segs))
	}
	for i, s := range segs {
		want := strings.TrimSpace(transcript.NormalizeEOL(s.Body, "\n"))
		if back.Cues[i].StartMs != s.StartMs || back.Cues[i].Text != want {
			return nil, fmt.Errorf("verify: cue %d differs", i+1)
		}
	}
	return &Result{
		Text: out, Name: name[:len(name)-3] + ".vtt", Cues: len(segs), DurationMs: durMs,
		Suspects: strings.Count(text, "[疑似"),
	}, nil
}
