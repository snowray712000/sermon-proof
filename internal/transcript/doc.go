// Package transcript parses sermon transcript markdown into raw blocks and
// editable segments, and renders it back byte-for-byte when nothing changed.
package transcript

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// segRe matches the first line of a segment: "**[mm:ss.mmm]**" or the plain
// "[mm:ss.mmm]" variant. Groups: 1 prefix, 2 time, 3 separator, 4 body.
var segRe = regexp.MustCompile(`^((?:\*\*)?\[(\d{1,3}(?::\d{1,3}){1,2}\.\d{3})\](?:\*\*)?)([ \t\x{3000}]*)(.*)$`)

// Block is either a raw span copied verbatim or one editable segment.
type Block struct {
	Segment bool
	// Raw is the exact original text of the block, including line endings.
	Raw string

	// Segment fields.
	ID       int
	StartMs  int
	Prefix   string // e.g. "**[00:00.000]**"
	Time     string // e.g. "00:00.000"
	Sep      string // whitespace between prefix and body
	Body     string // text after the separator; may contain internal EOLs
	OrigBody string // Body as parsed, to detect edits
	EOL      string // line ending of the segment's last line ("" at EOF)
}

// Document is a parsed transcript.
type Document struct {
	Blocks []Block
	// EOL is the dominant line ending ("\r\n" or "\n"), used for new text.
	EOL    string
	segIdx []int // segment ID -> index in Blocks
}

type line struct{ text, eol string }

func splitLines(s string) []line {
	var out []line
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, line{s, ""})
			break
		}
		text, eol := s[:i], "\n"
		if strings.HasSuffix(text, "\r") {
			text, eol = text[:len(text)-1], "\r\n"
		}
		out = append(out, line{text, eol})
		s = s[i+1:]
	}
	return out
}

// isContinuation reports whether a line directly after a segment line belongs
// to that segment (non-blank, not a heading/table/quote/list/fence/segment).
func isContinuation(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" || segRe.MatchString(text) {
		return false
	}
	switch t[0] {
	case '#', '|', '>':
		return false
	}
	return !strings.HasPrefix(t, "- ") && !strings.HasPrefix(t, "* ") && !strings.HasPrefix(t, "```")
}

// ParseTimeMs converts "mm:ss.mmm" or "h:mm:ss.mmm" to milliseconds.
func ParseTimeMs(s string) (int, error) {
	dot := strings.LastIndexByte(s, '.')
	if dot < 0 {
		return 0, fmt.Errorf("bad time %q", s)
	}
	ms, err := strconv.Atoi(s[dot+1:])
	if err != nil {
		return 0, fmt.Errorf("bad time %q", s)
	}
	total := 0
	for _, p := range strings.Split(s[:dot], ":") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return 0, fmt.Errorf("bad time %q", s)
		}
		total = total*60 + n
	}
	return total*1000 + ms, nil
}

// Parse splits text into blocks. It never fails: anything unrecognised is Raw.
func Parse(s string) *Document {
	d := &Document{}
	lines := splitLines(s)
	crlf := 0
	for _, l := range lines {
		if l.eol == "\r\n" {
			crlf++
		}
	}
	d.EOL = "\n"
	if crlf*2 > len(lines) {
		d.EOL = "\r\n"
	}

	for i := 0; i < len(lines); {
		m := segRe.FindStringSubmatch(lines[i].text)
		if m == nil {
			if n := len(d.Blocks); n > 0 && !d.Blocks[n-1].Segment {
				d.Blocks[n-1].Raw += lines[i].text + lines[i].eol
			} else {
				d.Blocks = append(d.Blocks, Block{Raw: lines[i].text + lines[i].eol})
			}
			i++
			continue
		}
		startMs, err := ParseTimeMs(m[2])
		if err != nil { // regex guarantees the shape; defensive only
			startMs = 0
		}
		var raw, body strings.Builder
		raw.WriteString(lines[i].text + lines[i].eol)
		body.WriteString(m[4])
		eol := lines[i].eol
		j := i + 1
		for j < len(lines) && isContinuation(lines[j].text) {
			body.WriteString(eol) // the previous line's terminator becomes internal
			body.WriteString(lines[j].text)
			raw.WriteString(lines[j].text + lines[j].eol)
			eol = lines[j].eol
			j++
		}
		id := len(d.segIdx)
		d.segIdx = append(d.segIdx, len(d.Blocks))
		d.Blocks = append(d.Blocks, Block{
			Segment: true, Raw: raw.String(), ID: id, StartMs: startMs,
			Prefix: m[1], Time: m[2], Sep: m[3],
			Body: body.String(), OrigBody: body.String(), EOL: eol,
		})
		i = j
	}
	return d
}

// NumSegments returns the number of editable segments.
func (d *Document) NumSegments() int { return len(d.segIdx) }

// Segment returns the segment with the given ID.
func (d *Document) Segment(id int) *Block { return &d.Blocks[d.segIdx[id]] }

// Segments returns all segments in order.
func (d *Document) Segments() []*Block {
	out := make([]*Block, len(d.segIdx))
	for i := range d.segIdx {
		out[i] = d.Segment(i)
	}
	return out
}

// Clone returns a deep copy.
func (d *Document) Clone() *Document {
	c := &Document{EOL: d.EOL}
	c.Blocks = append([]Block(nil), d.Blocks...)
	c.segIdx = append([]int(nil), d.segIdx...)
	return c
}

// NormalizeEOL converts any line ending in text to eol.
func NormalizeEOL(text, eol string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if eol != "\n" {
		text = strings.ReplaceAll(text, "\n", eol)
	}
	return text
}

// Apply sets new bodies (keyed by segment ID) and returns how many segments
// actually changed. Texts equal to the current body are ignored.
func (d *Document) Apply(edits map[int]string) (int, error) {
	for id := range edits {
		if id < 0 || id >= len(d.segIdx) {
			return 0, fmt.Errorf("no such segment %d", id)
		}
	}
	n := 0
	for id, text := range edits {
		b := d.Segment(id)
		text = NormalizeEOL(text, d.EOL)
		if text == b.Body {
			continue
		}
		b.Body = text
		n++
	}
	return n, nil
}

// Render writes the document. Untouched segments are emitted from their
// original bytes, which is what guarantees the round-trip.
func (d *Document) Render() string {
	var sb strings.Builder
	for i := range d.Blocks {
		b := &d.Blocks[i]
		if !b.Segment || b.Body == b.OrigBody {
			sb.WriteString(b.Raw)
			continue
		}
		sep := b.Sep
		if sep == "" && b.Body != "" {
			sep = " "
		}
		sb.WriteString(b.Prefix + sep + b.Body + b.EOL)
	}
	return sb.String()
}
