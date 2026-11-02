package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"sermonproof/internal/locate"
	"sermonproof/internal/transcript"
	"sermonproof/internal/vttexport"
)

// Session holds the one transcript currently open.
type Session struct {
	mu        sync.Mutex
	mdPath    string
	audioPath string
	doc       *transcript.Document
	hash      string // sha256 of the md bytes as last read or written
	review    *transcript.Review
}

// Seg is one segment as the front end sees it. Body uses "\n" for line breaks.
type Seg struct {
	ID      int    `json:"id"`
	StartMs int    `json:"startMs"`
	Time    string `json:"time"`
	Body    string `json:"body"`
	// EndMs is the cue's end (vtt only; 0 for md). ReadOnly marks a vtt cue
	// the tool will not edit (payload has WebVTT tags it cannot round-trip).
	EndMs    int  `json:"endMs,omitempty"`
	ReadOnly bool `json:"readOnly,omitempty"`
}

// State is the JSON returned by most endpoints.
type State struct {
	Open      bool               `json:"open"`
	MdPath    string             `json:"mdPath"`
	MdName    string             `json:"mdName"`
	AudioPath string             `json:"audioPath"`
	AudioName string             `json:"audioName"`
	HasAudio  bool               `json:"hasAudio"`
	HasOrig   bool               `json:"hasOrig"`
	Hash      string             `json:"hash"`
	EOL       string             `json:"eol"`
	Format    string             `json:"format"` // "md" or "vtt"
	Meta      []transcript.KV    `json:"meta"`   // vtt NOTE "key: value" lines, read only
	Segments  []Seg              `json:"segments"`
	Review    *transcript.Review `json:"review"`
}

// Edit replaces the body of one segment.
type Edit struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// ErrNotOpen is returned when no transcript is loaded.
var ErrNotOpen = errors.New("no transcript open")

// Open loads a transcript (.md or .vtt) and pairs its audio.
func (s *Session) Open(path string) error {
	if transcript.IsOrigName(filepath.Base(path)) {
		return errors.New("這是備份檔 (.orig.md / .orig.vtt)，請選原本的逐字稿")
	}
	if !transcript.IsTranscriptName(path) {
		return errors.New("不是 .md 或 .vtt 檔")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	doc, err := transcript.ParseByName(abs, string(b))
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mdPath = abs
	s.doc = doc
	s.hash = transcript.Hash(b)
	s.review = transcript.LoadReview(abs)
	s.audioPath = ""
	if a, ok := transcript.PairAudio(abs); ok {
		s.audioPath = a
		locate.Remember(a)
	}
	locate.Remember(abs)
	return nil
}

// SetAudio chooses the audio file explicitly.
func (s *Session) SetAudio(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if st, err := os.Stat(abs); err != nil || st.IsDir() {
		return fmt.Errorf("找不到音檔 %s", abs)
	}
	s.mu.Lock()
	s.audioPath = abs
	s.mu.Unlock()
	locate.Remember(abs)
	return nil
}

// Reload rereads the transcript from disk, discarding the in-memory copy.
func (s *Session) Reload() error {
	s.mu.Lock()
	p := s.mdPath
	s.mu.Unlock()
	if p == "" {
		return ErrNotOpen
	}
	return s.Open(p)
}

// AudioPath returns the current audio path ("" if none).
func (s *Session) AudioPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.audioPath
}

// Sibling is one transcript sitting in the same folder as the open one.
type Sibling struct {
	Name     string `json:"name"`
	HasAudio bool   `json:"hasAudio"` // an mp3 pairs with it
	HasOrig  bool   `json:"hasOrig"`  // it has been saved at least once (.orig.* exists)
	Current  bool   `json:"current"`
}

// isSiblingName reports whether name is a transcript the list should offer:
// .md / .vtt, not a backup, not a hidden file (macOS "._X.md" droppings).
func isSiblingName(name string) bool {
	return !strings.HasPrefix(name, ".") && transcript.IsTranscriptName(name) && !transcript.IsOrigName(name)
}

// Siblings lists the transcripts in the open transcript's folder, in directory
// order (the front end sorts). It is empty when nothing is open.
func (s *Session) Siblings() ([]Sibling, error) {
	s.mu.Lock()
	cur := s.mdPath
	s.mu.Unlock()
	if cur == "" {
		return []Sibling{}, nil
	}
	dir := filepath.Dir(cur)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []Sibling{}
	for _, e := range ents {
		if e.IsDir() || !isSiblingName(e.Name()) {
			continue
		}
		p := filepath.Join(dir, e.Name())
		_, hasAudio := transcript.PairAudio(p)
		_, oerr := os.Stat(transcript.OrigPath(p))
		out = append(out, Sibling{Name: e.Name(), HasAudio: hasAudio, HasOrig: oerr == nil, Current: p == cur})
	}
	return out, nil
}

// OpenSibling opens another transcript from the open one's folder. Only a bare
// file name is accepted, so the caller cannot reach outside that folder.
func (s *Session) OpenSibling(name string) error {
	s.mu.Lock()
	cur := s.mdPath
	s.mu.Unlock()
	if cur == "" {
		return ErrNotOpen
	}
	if name != filepath.Base(name) || !isSiblingName(name) {
		return errors.New("不是同資料夾的逐字稿：" + name)
	}
	return s.Open(filepath.Join(filepath.Dir(cur), name))
}

// SearchDirs lists folders worth walking to find a dropped file: where the
// open transcript and its audio live.
func (s *Session) SearchDirs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var dirs []string
	for _, p := range []string{s.mdPath, s.audioPath} {
		if p != "" {
			dirs = append(dirs, filepath.Dir(p))
		}
	}
	return dirs
}

// State snapshots the session.
func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateLocked()
}

func (s *Session) stateLocked() State {
	if s.doc == nil {
		return State{Segments: []Seg{}}
	}
	st := State{
		Open: true, MdPath: s.mdPath, MdName: filepath.Base(s.mdPath),
		AudioPath: s.audioPath, HasAudio: s.audioPath != "",
		Hash: s.hash, EOL: map[string]string{"\n": "lf", "\r\n": "crlf"}[s.doc.EOL],
		Format: s.doc.Format, Meta: s.doc.Meta,
		Segments: make([]Seg, 0, s.doc.NumSegments()), Review: s.review,
	}
	if st.HasAudio {
		st.AudioName = filepath.Base(s.audioPath)
	}
	if _, err := os.Stat(transcript.OrigPath(s.mdPath)); err == nil {
		st.HasOrig = true
	}
	for _, b := range s.doc.Segments() {
		st.Segments = append(st.Segments, Seg{
			ID: b.ID, StartMs: b.StartMs, Time: b.Time,
			Body:  strings.ReplaceAll(b.Body, "\r\n", "\n"),
			EndMs: b.EndMs, ReadOnly: b.ReadOnly,
		})
	}
	return st
}

// Save applies edits and writes the file. It returns the fresh state.
func (s *Session) Save(baseHash string, edits []Edit) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doc == nil {
		return State{}, ErrNotOpen
	}
	if baseHash != s.hash {
		return State{}, transcript.ErrConflict
	}
	m := make(map[int]string, len(edits))
	for _, e := range edits {
		m[e.ID] = e.Text
	}
	next := s.doc.Clone()
	n, err := next.Apply(m)
	if err != nil {
		return State{}, err
	}
	if n == 0 {
		return s.stateLocked(), nil
	}
	out := next.Render()
	doc, err := transcript.ParseByName(s.mdPath, out) // check it reads back before touching the disk
	if err != nil {
		return State{}, err
	}
	h, err := transcript.Save(s.mdPath, []byte(out), s.hash)
	if err != nil {
		return State{}, err
	}
	s.doc = doc
	s.hash = h
	return s.stateLocked(), nil
}

// SaveReview stores the checkbox sidecar.
func (s *Session) SaveReview(r *transcript.Review) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doc == nil {
		return ErrNotOpen
	}
	if err := transcript.SaveReview(s.mdPath, r); err != nil {
		return err
	}
	s.review = r
	return nil
}

// ErrVTTExists means the .vtt to write is already there and overwrite was not asked for.
var ErrVTTExists = errors.New("vtt already exists")

// ExportResult describes a finished (or skipped) .vtt export.
type ExportResult struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Cues      int    `json:"cues"`
	Replaced  bool   `json:"replaced"`  // an older .vtt was overwritten
	Unchanged bool   `json:"unchanged"` // the existing .vtt was already identical; nothing written
}

// ExportVTT writes the WebVTT copy of the open .md next to it (X-ver1.md ->
// X-ver1.vtt). The .md stays the source of truth: it is only read, and the
// export is of the saved file, so it must still match the loaded hash. An
// existing .vtt is replaced only with overwrite (atomically); status is the
// user's declaration for the NOTE block.
func (s *Session) ExportVTT(status string, overwrite bool) (ExportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doc == nil {
		return ExportResult{}, ErrNotOpen
	}
	if s.doc.Format != transcript.FormatMD {
		return ExportResult{}, errors.New("只有 .md 逐字稿能匯出成 .vtt")
	}
	status = strings.Join(strings.Fields(status), " ")
	if status == "" {
		return ExportResult{}, errors.New("請填 status（例如 proofread）")
	}
	b, err := os.ReadFile(s.mdPath)
	if err != nil {
		return ExportResult{}, err
	}
	if transcript.Hash(b) != s.hash {
		return ExportResult{}, transcript.ErrConflict
	}
	r, err := vttexport.FromMD(s.mdPath, b, vttexport.Options{Status: status})
	if err != nil {
		return ExportResult{}, err
	}
	dst := filepath.Join(filepath.Dir(s.mdPath), r.Name)
	res := ExportResult{Name: r.Name, Path: dst, Cues: r.Cues}
	if old, err := os.ReadFile(dst); err == nil {
		if string(old) == r.Text {
			res.Unchanged = true
			return res, nil
		}
		if !overwrite {
			return res, ErrVTTExists
		}
		res.Replaced = true
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(s.mdPath); err == nil {
		mode = st.Mode().Perm()
	}
	if err := transcript.WriteAtomic(dst, []byte(r.Text), mode); err != nil {
		return ExportResult{}, err
	}
	return res, nil
}
