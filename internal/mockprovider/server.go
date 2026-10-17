// Package mockprovider provides a localhost-only, single-process fault fixture.
// It is not an integration connector or a production data store.
package mockprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/rushikeshg25/runnelix/internal/spike"
)

type Action struct {
	ID    string         `json:"id"`
	Value map[string]any `json:"value"`
}
type Entry struct {
	Requests int      `json:"requests"`
	Actions  []Action `json:"actions"`
}
type Server struct {
	mu      sync.Mutex
	path    string
	entries map[string]Entry
}

func New(path string) (*Server, error) {
	s := &Server{path: path, entries: map[string]Entry{}}
	b, err := os.ReadFile(path)
	if err == nil {
		err = json.Unmarshal(b, &s.entries)
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if s.entries == nil {
		return nil, fmt.Errorf("invalid provider state")
	}
	return s, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s.entries)
	})
	mux.HandleFunc("POST /actions", s.action)
	return mux
}

func (s *Server) persist(entries map[string]Entry) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".provider-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = json.NewEncoder(f).Encode(entries); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), s.path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(s.path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *Server) record(req spike.Request) (Action, bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Copy the modified entry. A failed persistence operation must not publish
	// uncommitted in-memory state to a later HTTP request.
	next := make(map[string]Entry, len(s.entries)+1)
	for k, v := range s.entries {
		next[k] = v
	}
	e := next[req.RunID]
	e.Actions = append([]Action(nil), e.Actions...)
	e.Requests++
	transient := req.Fault == "transient-once" && e.Requests == 1
	duplicate := req.Mode == "safe" && len(e.Actions) > 0
	var action Action
	if duplicate {
		action = e.Actions[0]
	} else if !transient {
		action = Action{ID: fmt.Sprintf("%s-%d", req.RunID, len(e.Actions)+1), Value: req.Value}
		e.Actions = append(e.Actions, action)
	}
	next[req.RunID] = e
	if err := s.persist(next); err != nil {
		return Action{}, false, false, err
	}
	s.entries = next
	return action, duplicate, transient, nil
}

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil {
		http.Error(w, "request too large", http.StatusRequestEntityTooLarge)
		return
	}
	req, err := spike.DecodeRequest(b)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	action, duplicate, transient, err := s.record(req)
	if err != nil {
		http.Error(w, "persistence failed", http.StatusInternalServerError)
		return
	}
	if transient {
		http.Error(w, "retry later", http.StatusServiceUnavailable)
		return
	}
	if !duplicate && req.Fault == "disconnect" {
		conn, _, err := http.NewResponseController(w).Hijack()
		if err != nil {
			http.Error(w, "fault injection unavailable", 500)
			return
		}
		_ = conn.Close()
		return
	}
	if !duplicate && req.Fault == "hold" {
		<-r.Context().Done()
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(action)
}
