// Command mdtovtt converts sermon transcripts (X-ver1.md, X-ver1.orig.md) into
// WebVTT files, one cue per segment, with the background (book, passage,
// speaker, audio, status) in a leading NOTE block.
//
//	go run ./tools/mdtovtt -out DIR  FOLDER_OR_FILE...
//
// The .md is the source of truth and is only read; the .vtt is a derived copy.
// A target that already exists is reported and skipped unless -force is given
// (then it is regenerated, atomically). Each result is parsed back and compared
// with the source segments before it is written. The conversion itself lives in
// internal/vttexport, which the sermon-proof UI ("匯出 .vtt") shares.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"sermonproof/internal/transcript"
	"sermonproof/internal/vttexport"
)

func main() {
	out := flag.String("out", "", "output folder (required)")
	status := flag.String("status", "proofread", "status written for X-verN.md; declared by the user, never inferred")
	force := flag.Bool("force", false, "replace .vtt files that already exist (regenerated from the .md)")
	flag.Parse()
	if *out == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: mdtovtt [-force] [-status S] -out DIR FOLDER_OR_FILE...")
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
			if vttexport.NameRe.MatchString(filepath.Base(m)) {
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
		if msg, err := convert(f, *out, *status, *force); err != nil {
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

func convert(src, outDir, finalStatus string, force bool) (string, error) {
	raw, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	r, err := vttexport.FromMD(src, raw, vttexport.Options{Status: finalStatus})
	if err != nil {
		return "", err
	}
	dst := filepath.Join(outDir, r.Name)
	verb := "ok  "
	if _, err := os.Stat(dst); err == nil {
		if !force {
			return "SKIP " + r.Name + " (exists; -force replaces it)", nil
		}
		old, _ := os.ReadFile(dst)
		if string(old) == r.Text {
			return "same " + r.Name + " (unchanged)", nil
		}
		verb = "NEW "
	}
	if err := transcript.WriteAtomic(dst, []byte(r.Text), 0o644); err != nil {
		return "", err
	}
	extra := ""
	if r.Suspects > 0 && !transcript.IsOrigName(filepath.Base(src)) {
		extra = fmt.Sprintf("  (%d '[疑似' still in source, in table or text)", r.Suspects)
	}
	return fmt.Sprintf("%s %-36s %3d cues  %s%s", verb, r.Name, r.Cues, transcript.FormatVTTTime(r.DurationMs), extra), nil
}
