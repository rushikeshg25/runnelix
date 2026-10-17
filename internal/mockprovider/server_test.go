package mockprovider

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/rushikeshg25/runnelix/internal/spike"
)

func input(mode, fault string) []byte {
	b, _ := json.Marshal(spike.Request{ContractVersion: 1, RunID: "same-key", NodeVersion: "v1", Mode: mode, Fault: fault, Value: map[string]any{"message": "hello"}})
	return b
}

func TestConcurrentDeduplicationSurvivesReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/actions", bytes.NewReader(input("safe", "none"))))
			if w.Code != 200 {
				t.Errorf("status %d", w.Code)
			}
		})
	}
	wg.Wait()
	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	e := reloaded.entries["same-key"]
	if e.Requests != 20 || len(e.Actions) != 1 {
		t.Fatalf("entry %+v", e)
	}
}

func TestUnsafeWritesRepeatAndTransientDoesNotCommit(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/actions", bytes.NewReader(input("unsafe", "transient-once"))))
		want := 200
		if i == 0 {
			want = 503
		}
		if w.Code != want {
			t.Fatalf("status %d, want %d", w.Code, want)
		}
	}
	if len(s.entries["same-key"].Actions) != 2 {
		t.Fatal("unsafe writes were incorrectly deduplicated")
	}
}

func TestDisconnectAfterDurableCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(s.Handler())
	defer h.Close()
	response, err := http.Post(h.URL+"/actions", "application/json", bytes.NewReader(input("safe", "disconnect")))
	if err == nil {
		response.Body.Close()
		t.Fatal("expected a lost response")
	}
	reloaded, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.entries["same-key"].Actions) != 1 {
		t.Fatal("remote commit missing after lost response")
	}
}

func TestCorruptStateRejected(t *testing.T) {
	if _, err := New(t.TempDir()); err == nil {
		t.Fatal("accepted unreadable state path")
	}
}
