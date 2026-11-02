package transcript

import (
	"fmt"
	"strings"
)

// KV is one "key: value" line of the metadata NOTE block.
type KV struct{ Key, Value string }

// Cue is one subtitle cue. A cue is one transcript segment; its end is the
// next cue's start (the last one ends at the audio's end).
type Cue struct {
	ID      string
	StartMs int
	EndMs   int
	Text    string // unescaped; lines separated by "\n"
}

// VTT is a WebVTT sermon transcript: ordered metadata plus cues.
type VTT struct {
	Meta []KV
	Cues []Cue
}

var vttEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
var vttUnescape = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&")

// FromDocument turns the segments of a parsed .md into cues. Segment text is
// trimmed and its line endings normalised to "\n"; nothing else is changed.
// durationMs is the audio length (used for the last cue's end; <=0 = unknown).
func FromDocument(d *Document, meta []KV, durationMs int) *VTT {
	v := &VTT{Meta: meta}
	segs := d.Segments()
	for i, s := range segs {
		c := Cue{
			ID:      fmt.Sprintf("s%03d", i+1),
			StartMs: s.StartMs,
			Text:    strings.TrimSpace(NormalizeEOL(s.Body, "\n")),
		}
		if i+1 < len(segs) {
			c.EndMs = segs[i+1].StartMs
		} else {
			c.EndMs = durationMs
		}
		if c.EndMs <= c.StartMs {
			c.EndMs = c.StartMs + 1000
		}
		v.Cues = append(v.Cues, c)
	}
	return v
}

// FormatVTTTime renders ms as mm:ss.mmm (hh:mm:ss.mmm from one hour).
func FormatVTTTime(ms int) string {
	h, m, s, f := ms/3600000, ms/60000%60, ms/1000%60, ms%1000
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, f)
	}
	return fmt.Sprintf("%02d:%02d.%03d", m, s, f)
}

// String renders the file with "\n" line endings and a trailing newline.
func (v *VTT) String() string {
	var sb strings.Builder
	sb.WriteString("WEBVTT\n")
	if len(v.Meta) > 0 {
		sb.WriteString("\nNOTE\n")
		for _, kv := range v.Meta {
			val := strings.Join(strings.Fields(strings.ReplaceAll(kv.Value, "-->", "->")), " ")
			sb.WriteString(kv.Key + ": " + val + "\n")
		}
	}
	for _, c := range v.Cues {
		sb.WriteString("\n")
		if c.ID != "" {
			sb.WriteString(c.ID + "\n")
		}
		sb.WriteString(FormatVTTTime(c.StartMs) + " --> " + FormatVTTTime(c.EndMs) + "\n")
		sb.WriteString(vttEscape.Replace(c.Text) + "\n")
	}
	return sb.String()
}

// ParseVTT reads a file made by String (or any plain WebVTT: cue settings
// after the end time are ignored, STYLE/REGION/other NOTE blocks skipped).
// The first NOTE block, if it holds "key: value" lines, becomes Meta.
func ParseVTT(s string) (*VTT, error) {
	s = strings.TrimPrefix(s, "\xef\xbb\xbf")
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if s != "WEBVTT" && !strings.HasPrefix(s, "WEBVTT\n") && !strings.HasPrefix(s, "WEBVTT ") {
		return nil, fmt.Errorf("not a WebVTT file (missing WEBVTT header)")
	}
	v := &VTT{}
	haveMeta := false
	blocks := strings.Split(s, "\n\n")
	for i, blk := range blocks {
		blk = strings.Trim(blk, "\n")
		if blk == "" {
			continue
		}
		lines := strings.Split(blk, "\n")
		if i == 0 { // header block (may carry a title); cues need a blank line first
			continue
		}
		ti := -1
		for j, l := range lines {
			if strings.Contains(l, "-->") {
				ti = j
				break
			}
		}
		if ti < 0 || ti > 1 {
			if lines[0] == "NOTE" && !haveMeta {
				haveMeta = true
				for _, l := range lines[1:] {
					if k, val, ok := strings.Cut(l, ": "); ok {
						v.Meta = append(v.Meta, KV{k, val})
					}
				}
			}
			continue
		}
		c := Cue{}
		if ti == 1 {
			c.ID = lines[0]
		}
		a, b, _ := strings.Cut(lines[ti], "-->")
		b = strings.Fields(b + " ")[0]
		var err error
		if c.StartMs, err = ParseTimeMs(strings.TrimSpace(a)); err != nil {
			return nil, err
		}
		if c.EndMs, err = ParseTimeMs(b); err != nil {
			return nil, err
		}
		c.Text = vttUnescape.Replace(strings.Join(lines[ti+1:], "\n"))
		v.Cues = append(v.Cues, c)
	}
	return v, nil
}
