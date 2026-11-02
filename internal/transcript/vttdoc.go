package transcript

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// A .vtt file is read into the same Document as a .md: raw spans copied
// verbatim plus one editable segment per cue. A segment's Prefix is the cue's
// id line (if any) and timing line, with their line endings; Body is the
// payload with "&amp; &lt; &gt;" undone. Untouched cues are written from their
// original bytes, so reading and writing back is byte-exact. Only the payload
// of a cue the user edited is regenerated (escaped again).

// vttTimingRe is a cue timing line: "start --> end" plus optional settings.
var vttTimingRe = regexp.MustCompile(`^((?:\d{2,}:)?\d{2}:\d{2}\.\d{3})[ \t]+-->[ \t]+((?:\d{2,}:)?\d{2}:\d{2}\.\d{3})(?:[ \t].*)?$`)

// vttHeaderLine reports whether text (BOM already allowed) is the WEBVTT signature line.
func vttHeaderLine(text string) bool {
	text = strings.TrimPrefix(text, "\xef\xbb\xbf")
	return text == "WEBVTT" || strings.HasPrefix(text, "WEBVTT ") || strings.HasPrefix(text, "WEBVTT\t")
}

// ErrNotVTT is returned by ParseVTTDoc when the WEBVTT signature is missing.
var ErrNotVTT = errors.New("not a WebVTT file (missing WEBVTT header)")

// ParseVTTDoc splits a WebVTT file into raw blocks and editable cue segments.
// Anything that is not a well-formed cue (header, NOTE, STYLE, REGION, odd
// blocks, stray blank lines) stays raw. It fails only when the file does not
// start with WEBVTT.
func ParseVTTDoc(s string) (*Document, error) {
	lines := splitLines(s)
	if len(lines) == 0 || !vttHeaderLine(lines[0].text) {
		return nil, ErrNotVTT
	}
	d := &Document{Format: FormatVTT, EOL: dominantEOL(lines)}
	addRaw := func(l ...line) {
		var sb strings.Builder
		for _, x := range l {
			sb.WriteString(x.text + x.eol)
		}
		if n := len(d.Blocks); n > 0 && !d.Blocks[n-1].Segment {
			d.Blocks[n-1].Raw += sb.String()
		} else {
			d.Blocks = append(d.Blocks, Block{Raw: sb.String()})
		}
	}

	i := 0
	for i < len(lines) && lines[i].text != "" { // header block
		addRaw(lines[i])
		i++
	}
	for i < len(lines) {
		if lines[i].text == "" {
			addRaw(lines[i])
			i++
			continue
		}
		j := i
		for j < len(lines) && lines[j].text != "" {
			j++
		}
		d.addVTTBlock(lines[i:j], addRaw)
		i = j
	}
	return d, nil
}

// addVTTBlock classifies one blank-line-delimited block.
func (d *Document) addVTTBlock(blk []line, addRaw func(...line)) {
	first := blk[0].text
	if first == "NOTE" || strings.HasPrefix(first, "NOTE ") || strings.HasPrefix(first, "NOTE\t") {
		if d.Meta == nil && len(d.segIdx) == 0 {
			for _, l := range blk[1:] {
				if k, v, ok := strings.Cut(l.text, ": "); ok {
					d.Meta = append(d.Meta, KV{k, v})
				}
			}
		}
		addRaw(blk...)
		return
	}
	ti := -1
	switch {
	case vttTimingRe.MatchString(blk[0].text):
		ti = 0
	case len(blk) > 1 && !strings.Contains(blk[0].text, "-->") && vttTimingRe.MatchString(blk[1].text):
		ti = 1
	}
	if ti < 0 {
		addRaw(blk...)
		return
	}
	payload := blk[ti+1:]
	for _, l := range payload {
		if strings.Contains(l.text, "-->") { // would start another cue in a player
			addRaw(blk...)
			return
		}
	}

	m := vttTimingRe.FindStringSubmatch(blk[ti].text)
	startMs, err1 := ParseTimeMs(m[1])
	endMs, err2 := ParseTimeMs(m[2])
	if err1 != nil || err2 != nil { // regex guarantees the shape; defensive only
		addRaw(blk...)
		return
	}
	var prefix, raw, body strings.Builder
	for _, l := range blk[:ti+1] {
		prefix.WriteString(l.text + l.eol)
	}
	raw.WriteString(prefix.String())
	eol := blk[ti].eol // an empty payload: the timing line's terminator
	for k, l := range payload {
		if k > 0 {
			body.WriteString(eol) // the previous line's terminator becomes internal
		}
		body.WriteString(l.text)
		raw.WriteString(l.text + l.eol)
		eol = l.eol
	}
	text := vttUnescape.Replace(body.String())
	id := len(d.segIdx)
	d.segIdx = append(d.segIdx, len(d.Blocks))
	d.Blocks = append(d.Blocks, Block{
		Segment: true, Raw: raw.String(), ID: id, StartMs: startMs, EndMs: endMs,
		Prefix: prefix.String(), Time: m[1], Body: text, OrigBody: text, EOL: eol,
		ReadOnly: vttEscape.Replace(text) != body.String(),
	})
}

// checkVTTEdit refuses edits that could not be written back as a valid cue.
func checkVTTEdit(b *Block, text string) error {
	if b.ReadOnly {
		return fmt.Errorf("字幕 %s 含 WebVTT 標籤或未跳脫的 & < >，本工具不編輯這一段", b.Time)
	}
	if text != "" {
		for _, l := range strings.Split(text, "\n") {
			if l == "" {
				return fmt.Errorf("字幕 %s：段落內不能有空行（WebVTT 會把空行當成字幕結束）", b.Time)
			}
		}
	}
	return nil
}

// renderVTTCue writes an edited cue: its id/timing lines as read, then the new
// payload escaped. An emptied payload is dropped entirely.
func (d *Document) renderVTTCue(b *Block) string {
	prefix := b.Prefix
	if b.Body == "" {
		return prefix
	}
	if !strings.HasSuffix(prefix, "\n") { // timing line was the unterminated last line of the file
		prefix += d.EOL
	}
	return prefix + vttEscape.Replace(b.Body) + b.EOL
}

// ParseByName parses a transcript according to its file extension (.vtt or .md).
func ParseByName(name, text string) (*Document, error) {
	if IsVTTName(name) {
		return ParseVTTDoc(text)
	}
	return Parse(text), nil
}
