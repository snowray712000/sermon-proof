// Package server is the loopback-only HTTP API and static front end.
package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"

	"sermonproof/internal/audio"
	"sermonproof/internal/filedlg"
	"sermonproof/internal/locate"
	"sermonproof/internal/transcript"
	"sermonproof/web"
)

// Server serves the UI and API for one Session.
type Server struct {
	Sess  *Session
	token string
	port  string // set by SetPort once the listener is bound
	plans audio.Cache
}

// New creates a server with a fresh random token.
func New(sess *Session) *Server {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return &Server{Sess: sess, token: hex.EncodeToString(b)}
}

// SetPort records the bound port, used for Host / Origin checks.
func (s *Server) SetPort(p string) { s.port = p }

// Token is the secret embedded in the page and required by /api/*.
func (s *Server) Token() string { return s.token }

// Handler builds the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.Handle("GET /app.js", http.FileServerFS(web.FS))
	mux.Handle("GET /style.css", http.FileServerFS(web.FS))

	mux.HandleFunc("GET /api/state", s.api(s.handleState))
	mux.HandleFunc("POST /api/pick-md", s.api(s.handlePickMD))
	mux.HandleFunc("POST /api/pick-audio", s.api(s.handlePickAudio))
	mux.HandleFunc("POST /api/open-dropped", s.api(s.handleOpenDropped))
	mux.HandleFunc("POST /api/reload", s.api(s.handleReload))
	mux.HandleFunc("POST /api/save", s.api(s.handleSave))
	mux.HandleFunc("PUT /api/review", s.api(s.handleReview))
	mux.HandleFunc("GET /api/audio", s.api(s.handleAudio))
	return s.guard(mux)
}

// guard rejects requests whose Host or Origin is not our own loopback address,
// which blocks DNS-rebinding and cross-site requests from other web pages.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "127.0.0.1:"+s.port && r.Host != "localhost:"+s.port {
			http.Error(w, "bad host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host {
			http.Error(w, "bad origin", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// api wraps a handler with the token check (header, or ?t= for <audio>).
func (s *Server) api(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := r.Header.Get("X-Token")
		if t == "" {
			t = r.URL.Query().Get("t")
		}
		if subtle.ConstantTimeCompare([]byte(t), []byte(s.token)) != 1 {
			writeErr(w, http.StatusForbidden, "bad token")
			return
		}
		if r.Method != http.MethodGet && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			writeErr(w, http.StatusUnsupportedMediaType, "json only")
			return
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	b, err := fs.ReadFile(web.FS, "index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := strings.Replace(string(b), "__TOKEN__", s.token, 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte(page))
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Sess.State())
}

func (s *Server) handlePickMD(w http.ResponseWriter, r *http.Request) {
	p, err := filedlg.Pick("md")
	if errors.Is(err, filedlg.ErrCancelled) {
		writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Sess.OpenMD(p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Sess.State())
}

func (s *Server) handlePickAudio(w http.ResponseWriter, r *http.Request) {
	p, err := filedlg.Pick("audio")
	if errors.Is(err, filedlg.ErrCancelled) {
		writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Sess.SetAudio(p); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Sess.State())
}

// handleOpenDropped opens files dragged onto the page. The browser only reports
// name / size / mtime, so each is resolved to a real path by internal/locate.
// A dropped .md is opened first (which pairs its own audio); a dropped audio
// file then overrides the pairing, or is applied to the transcript already open.
func (s *Server) handleOpenDropped(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Files []struct {
			Name    string `json:"name"`
			Size    int64  `json:"size"`
			MtimeMs int64  `json:"mtimeMs"`
		} `json:"files"`
		URIs []string `json:"uris"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var mdPath, audioPath string
	for _, f := range req.Files {
		kind := dropKind(f.Name)
		if kind == "" {
			writeErr(w, http.StatusBadRequest, "不支援的檔案："+f.Name+"（只收 .md 與 mp3／m4a／wav）")
			return
		}
		if (kind == "md" && mdPath != "") || (kind == "audio" && audioPath != "") {
			writeErr(w, http.StatusBadRequest, "一次只能拖入一個逐字稿和一個音檔")
			return
		}
		p, err := locate.Find(locate.Query{Name: f.Name, Size: f.Size, MtimeMs: f.MtimeMs, URIs: req.URIs}, s.Sess.SearchDirs())
		if err != nil {
			writeErr(w, http.StatusNotFound, f.Name+"：\n"+err.Error())
			return
		}
		if kind == "md" {
			mdPath = p
		} else {
			audioPath = p
		}
	}
	if mdPath == "" && audioPath == "" {
		writeErr(w, http.StatusBadRequest, "沒有收到檔案")
		return
	}
	if mdPath == "" && !s.Sess.State().Open {
		writeErr(w, http.StatusBadRequest, "請先拖入逐字稿 (.md)")
		return
	}
	if mdPath != "" {
		if err := s.Sess.OpenMD(mdPath); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if audioPath != "" {
		if err := s.Sess.SetAudio(audioPath); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	writeJSON(w, http.StatusOK, s.Sess.State())
}

func dropKind(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md":
		return "md"
	case ".mp3", ".m4a", ".wav":
		return "audio"
	}
	return ""
}

func (s *Server) handleReload(w http.ResponseWriter, r *http.Request) {
	if err := s.Sess.Reload(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.Sess.State())
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BaseHash string `json:"baseHash"`
		Edits    []Edit `json:"edits"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	st, err := s.Sess.Save(req.BaseHash, req.Edits)
	switch {
	case errors.Is(err, transcript.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "磁碟上的檔案已被外部改動", "code": "conflict"})
	case err != nil:
		writeErr(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSON(w, http.StatusOK, st)
	}
}

func (s *Server) handleReview(w http.ResponseWriter, r *http.Request) {
	var rv transcript.Review
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&rv); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Sess.SaveReview(&rv); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAudio(w http.ResponseWriter, r *http.Request) {
	p := s.Sess.AudioPath()
	if p == "" {
		writeErr(w, http.StatusNotFound, "no audio")
		return
	}
	plan, err := s.plans.Get(p)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	src, err := plan.Open(p)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	defer src.Close()
	http.ServeContent(w, r, p, plan.ModTime(), src) // handles Range
}
