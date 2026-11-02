// Command mdtovtt converts sermon transcripts (X-ver1.md, X-ver1.orig.md) into
// WebVTT files, one cue per segment, with the background (book, passage,
// speaker, audio, status) in a leading NOTE block.
//
//	go run ./tools/mdtovtt -out DIR  FOLDER_OR_FILE...
//
// Sources are only read. Nothing existing is overwritten: a target that is
// already there is reported and skipped. Each result is parsed back and
// compared with the source segments before it is written.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"sermonproof/internal/audio"
	"sermonproof/internal/transcript"
)

var (
	baseRe   = regexp.MustCompile(`^(N\d+)_(\d+)_(\d+)_(\d+)_(\d+)-ver(\d+)(\.orig)?\.md$`)
	folderRe = regexp.MustCompile(`^(N\d+)_(.+)$`)
)

func main() {
	out := flag.String("out", "", "output folder (required)")
	status := flag.String("status", "proofread", "status written for X-verN.md; declared by the user, never inferred")
	flag.Parse()
	if *out == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: mdtovtt -out DIR FOLDER_OR_FILE...")
		os.Exit(2)
	}
	var files []string
	for _, a := range flag.Args() {
		st, err := os.Stat(a)
		if err != nil {
			fatal(err)
		}
		if !st.IsDir() {
			files = append(files, a)
			continue
		}
		ms, _ := filepath.Glob(filepath.Join(a, "N*-ver*.md"))
		for _, m := range ms {
			if baseRe.MatchString(filepath.Base(m)) {
				files = append(files, m)
			}
		}
	}
	sort.Strings(files)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fatal(err)
	}
	failed := 0
	for _, f := range files {
		if msg, err := convert(f, *out, *status); err != nil {
			fmt.Printf("FAIL %s: %v\n", filepath.Base(f), err)
			failed++
		} else {
			fmt.Println(msg)
		}
	}
	fmt.Printf("%d files, %d failed\n", len(files), failed)
	if failed > 0 {
		os.Exit(1)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func convert(src, outDir, finalStatus string) (string, error) {
	name := filepath.Base(src)
	m := baseRe.FindStringSubmatch(name)
	if m == nil {
		return "", fmt.Errorf("name does not look like N59_a_b_c_d-ver1[.orig].md")
	}
	code, c1, v1, c2, v2, ver, isOrig := m[1], m[2], m[3], m[4], m[5], m[6], m[7] != ""
	stem := strings.TrimSuffix(strings.TrimSuffix(name, ".md"), ".orig")
	base := strings.SplitN(stem, "-ver", 2)[0]

	raw, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	text := strings.TrimPrefix(string(raw), "\xef\xbb\xbf")
	d := transcript.Parse(text)
	if d.NumSegments() == 0 {
		return "", fmt.Errorf("no segments")
	}

	dir := filepath.Dir(src)
	mp3 := base + ".mp3"
	durMs := 0
	if dur, err := audio.Duration(filepath.Join(dir, mp3)); err == nil {
		durMs = int(dur.Milliseconds())
	} else {
		mp3 = ""
	}

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

	var meta []transcript.KV
	add := func(k, v string) {
		if v != "" {
			meta = append(meta, transcript.KV{Key: k, Value: v})
		}
	}
	if book != "" {
		add("book", book+" ("+code+")")
	} else {
		add("book", code)
	}
	add("passage", passage)
	add("speaker", speaker)
	add("audio", mp3)
	if durMs > 0 {
		add("duration", transcript.FormatVTTTime(durMs))
	}
	add("version", "ver"+ver)
	st := finalStatus
	if isOrig {
		st = "machine-draft"
		add("status", st)
	} else {
		add("status", st)
		add("draft", stem+".orig.vtt")
	}
	add("timing", "每段開始時間為自動對齊，未經人工校準；結束時間為下一段開始時間")
	note := "「……」與「...」表示停頓、轉折或未完成句，不是刪節"
	if strings.Contains(text, "???") {
		note += "；「???」表示未知"
	}
	add("notation", note)
	add("converted-from", name)

	v := transcript.FromDocument(d, meta, durMs)
	outText := v.String()

	// Verify: parse back and compare with the source segments.
	back, err := transcript.ParseVTT(outText)
	if err != nil {
		return "", err
	}
	segs := d.Segments()
	if len(back.Cues) != len(segs) {
		return "", fmt.Errorf("verify: %d cues for %d segments", len(back.Cues), len(segs))
	}
	for i, s := range segs {
		want := strings.TrimSpace(transcript.NormalizeEOL(s.Body, "\n"))
		if back.Cues[i].StartMs != s.StartMs || back.Cues[i].Text != want {
			return "", fmt.Errorf("verify: cue %d differs", i+1)
		}
	}

	dst := filepath.Join(outDir, strings.TrimSuffix(name, ".md")+".vtt")
	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "SKIP " + filepath.Base(dst) + " (exists)", nil
		}
		return "", err
	}
	if _, err := f.WriteString(outText); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	left := strings.Count(text, "[疑似")
	extra := ""
	if left > 0 && !isOrig {
		extra = fmt.Sprintf("  (%d '[疑似' still in source, in table or text)", left)
	}
	return fmt.Sprintf("ok   %-36s %3d cues  %s%s", filepath.Base(dst), len(segs), transcript.FormatVTTTime(durMs), extra), nil
}
