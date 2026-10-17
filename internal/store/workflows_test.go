package store

import (
	"context"
	"errors"
	"sync"
	"testing"
)

var graph = []byte(`{"formatVersion":1,"nodes":[{"id":"start","type":"trigger.manual","version":1,"config":{}},{"id":"echo","type":"mock.echo","version":1,"config":{}}],"edges":[{"from":{"node":"start","port":"out"},"to":{"node":"echo","port":"in"}}]}`)

func create(t *testing.T, s *Store, ws string) Workflow {
	t.Helper()
	w, err := s.CreateWorkflow(context.Background(), ws, "Example", graph)
	if err != nil {
		t.Fatal(err)
	}
	return w
}
func TestDraftConcurrencyAndScope(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws := workspace(t, s)
	other := workspace(t, s)
	w := create(t, s, ws)
	if _, err := s.GetWorkflow(ctx, other, w.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Go(func() { _, err := s.SaveDraft(ctx, ws, w.ID, 1, graph); results <- err })
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		if err == nil {
			wins++
		} else if errors.Is(err, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("wins=%d conflicts=%d", wins, conflicts)
	}
	if _, err := s.SaveDraft(ctx, other, w.ID, 2, graph); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx, other, w.ID, 2); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
func TestPublicationImmutableAndPinned(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws := workspace(t, s)
	w := create(t, s, ws)
	v, err := s.Publish(ctx, ws, w.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Publish(ctx, ws, w.ID, 1)
	if err != nil || again.ID != v.ID {
		t.Fatal("publication not idempotent", err)
	}
	for _, query := range []string{"UPDATE workflow_versions SET digest=repeat('a',64) WHERE id=$1", "DELETE FROM workflow_versions WHERE id=$1"} {
		if _, err = s.Pool.Exec(ctx, query, v.ID); err == nil {
			t.Fatal("mutable published version")
		}
	}
	if _, err = s.SaveDraft(ctx, ws, w.ID, 1, graph); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx, ws, w.ID, 1); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	v2, err := s.Publish(ctx, ws, w.ID, 2)
	if err != nil || v2.ID == v.ID || v2.Sequence != 2 {
		t.Fatal(err)
	}
	old, err := s.GetVersion(ctx, ws, w.ID, v.ID)
	if err != nil || old.Digest != v.Digest {
		t.Fatal("old version changed")
	}
	other := workspace(t, s)
	w2 := create(t, s, other)
	if _, err = s.Pool.Exec(ctx, "UPDATE workflows SET active_version_id=$3 WHERE workspace_id=$1 AND id=$2", other, w2.ID, v.ID); err == nil {
		t.Fatal("cross-workspace version allowed")
	}
}
func TestInvalidDraftCanSaveButCannotPublish(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws := workspace(t, s)
	bad := []byte(`{"formatVersion":1,"nodes":[{"id":"echo","type":"mock.echo","version":1,"config":{}}],"edges":[]}`)
	w, err := s.CreateWorkflow(ctx, ws, "Incomplete", bad)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx, ws, w.ID, 1); err == nil {
		t.Fatal("invalid graph published")
	}
	w, err = s.GetWorkflow(ctx, ws, w.ID)
	if err != nil || w.ActiveVersionID != nil {
		t.Fatal("failed publish mutated active pointer")
	}
}
