package server

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"sermonproof/internal/transcript"
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

// OpenMD loads a transcript and pairs its audio.
func (s *Session) OpenMD(path string) error {
	if transcript.IsOrigName(filepath.Base(path)) {
		return errors.New("這是備份檔 (.orig.md)，請選原本的 .md")
	}
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return errors.New("不是 .md 檔")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mdPath = abs
	s.doc = transcript.Parse(string(b))
	s.hash = transcript.Hash(b)
	s.review = transcript.LoadReview(abs)
	s.audioPath = ""
	if a, ok := transcript.PairAudio(abs); ok {
		s.audioPath = a
	}
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
	return nil
}

// Reload rereads the md from disk, discarding the in-memory copy.
func (s *Session) Reload() error {
	s.mu.Lock()
	p := s.mdPath
	s.mu.Unlock()
	if p == "" {
		return ErrNotOpen
	}
	return s.OpenMD(p)
}

// AudioPath returns the current audio path ("" if none).
func (s *Session) AudioPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.audioPath
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
			Body: strings.ReplaceAll(b.Body, "\r\n", "\n"),
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
	h, err := transcript.Save(s.mdPath, []byte(out), s.hash)
	if err != nil {
		return State{}, err
	}
	s.doc = transcript.Parse(out)
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
