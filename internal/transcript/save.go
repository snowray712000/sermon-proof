package transcript

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ErrConflict means the file on disk is not the one we loaded.
var ErrConflict = errors.New("file changed on disk since it was loaded")

// Hash returns the hex sha256 of b.
func Hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// OrigPath is where the first-save backup lives: X.md -> X.orig.md.
func OrigPath(md string) string {
	return strings.TrimSuffix(md, filepath.Ext(md)) + ".orig" + filepath.Ext(md)
}

// ReviewPath is the sidecar holding checkbox state: X.md -> X.review.json.
func ReviewPath(md string) string {
	return strings.TrimSuffix(md, filepath.Ext(md)) + ".review.json"
}

// Save writes data over path if the file still hashes to expectHash. Before
// the first write it copies the on-disk file to X.orig.md (never overwritten),
// then writes a temp file in the same directory and renames it into place.
func Save(path string, data []byte, expectHash string) (string, error) {
	cur, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if Hash(cur) != expectHash {
		return "", ErrConflict
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	mode := st.Mode().Perm()

	f, err := os.OpenFile(OrigPath(path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	switch {
	case err == nil:
		_, werr := f.Write(cur)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return "", werr
		}
	case errors.Is(err, fs.ErrExist):
		// keep the existing backup untouched
	default:
		return "", err
	}

	if err := atomicWrite(path, data, mode); err != nil {
		return "", err
	}
	return Hash(data), nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	fail := func(err error) error {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	if err := tmp.Chmod(mode); err != nil {
		return fail(err)
	}
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Review is the sidecar state: which segments / suspect marks the user ticked.
// Keys are the marker's start time in ms, as decimal strings.
type Review struct {
	Segments map[string]bool `json:"segments"`
	Suspects map[string]bool `json:"suspects"`
}

func (r *Review) normalize() {
	if r.Segments == nil {
		r.Segments = map[string]bool{}
	}
	if r.Suspects == nil {
		r.Suspects = map[string]bool{}
	}
}

// LoadReview reads the sidecar; a missing or unreadable file is an empty review.
func LoadReview(md string) *Review {
	r := &Review{}
	if b, err := os.ReadFile(ReviewPath(md)); err == nil {
		_ = json.Unmarshal(b, r)
	}
	r.normalize()
	return r
}

// SaveReview writes the sidecar atomically.
func SaveReview(md string, r *Review) error {
	r.normalize()
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(ReviewPath(md), append(b, '\n'), 0o644)
}
