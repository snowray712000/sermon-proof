package transcript

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var verSuffixRe = regexp.MustCompile(`-ver\d+$`)

// IsOrigName reports whether name is a backup made by this tool ("X.orig.md").
func IsOrigName(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".orig.md")
}

// PairAudio finds the mp3 that belongs to a transcript by stripping the known
// suffixes one at a time: N59_x-ver1-manual.md tries N59_x-ver1-manual.mp3,
// N59_x-ver1.mp3, then N59_x.mp3.
func PairAudio(mdPath string) (string, bool) {
	dir := filepath.Dir(mdPath)
	base := filepath.Base(mdPath)
	if strings.EqualFold(filepath.Ext(base), ".md") {
		base = base[:len(base)-3]
	}
	for {
		for _, ext := range []string{".mp3", ".MP3"} {
			p := filepath.Join(dir, base+ext)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, true
			}
		}
		next := base
		switch {
		case strings.HasSuffix(base, ".orig"):
			next = strings.TrimSuffix(base, ".orig")
		case strings.HasSuffix(base, "-manual"):
			next = strings.TrimSuffix(base, "-manual")
		case verSuffixRe.MatchString(base):
			next = verSuffixRe.ReplaceAllString(base, "")
		}
		if next == base || next == "" {
			return "", false
		}
		base = next
	}
}
