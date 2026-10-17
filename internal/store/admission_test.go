package store

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func published(t *testing.T, s *Store) (string, Workflow, Version) {
	t.Helper()
	ws := workspace(t, s)
	w := create(t, s, ws)
	v, err := s.Publish(context.Background(), ws, w.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	return ws, w, v
}
func TestConcurrentAdmissionAndPinning(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, v := published(t, s)
	var wg sync.WaitGroup
	ids := make(chan string, 12)
	errs := make(chan error, 12)
	for range 12 {
		wg.Go(func() { r, err := s.Admit(ctx, ws, w.ID, "event_1", []byte(`{"a":1,"b":2}`)); ids <- r.ID; errs <- err })
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	first := ""
	for id := range ids {
		if first == "" {
			first = id
		}
		if first != id {
			t.Fatal("duplicate intended runs")
		}
	}
	r, err := s.Admit(ctx, ws, w.ID, "event_1", []byte(`{ "b":2, "a":1 }`))
	if err != nil || r.ID != first {
		t.Fatal("canonical replay failed", err)
	}
	if _, err = s.Admit(ctx, ws, w.ID, "event_1", []byte(`{"a":2}`)); !errors.Is(err, ErrConflict) {
		t.Fatal("changed payload accepted", err)
	}
	if _, err = s.SaveDraft(ctx, ws, w.ID, 1, graph); err != nil {
		t.Fatal(err)
	}
	v2, err := s.Publish(ctx, ws, w.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.Admit(ctx, ws, w.ID, "event_1", []byte(`{"a":1,"b":2}`))
	if err != nil || replay.VersionID != v.ID {
		t.Fatal("replay moved version", err)
	}
	next, err := s.Admit(ctx, ws, w.ID, "event_2", []byte(`{}`))
	if err != nil || next.VersionID != v2.ID {
		t.Fatal("new admission not pinned", err)
	}
	other := workspace(t, s)
	if _, err = s.GetRun(ctx, other, r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("run scope leak", err)
	}
	if _, err = s.Admit(ctx, other, w.ID, "event_1", []byte(`{}`)); !errors.Is(err, ErrNotFound) {
		t.Fatal("admission scope leak", err)
	}
	var runs, inbox, outbox int
	if err = s.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM runs WHERE workspace_id=$1),(SELECT count(*) FROM trigger_inbox WHERE workspace_id=$1),(SELECT count(*) FROM start_outbox WHERE workspace_id=$1)`, ws).Scan(&runs, &inbox, &outbox); err != nil || runs != 2 || inbox != 2 || outbox != 2 {
		t.Fatalf("counts %d %d %d error %v", runs, inbox, outbox, err)
	}
}
func TestAdmissionAtomicRollback(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, _ := published(t, s)
	// Force the final insert to fail only for this isolated workspace; earlier inserts must roll back.
	_, err := s.Pool.Exec(ctx, `CREATE FUNCTION reject_fixture_outbox() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.workspace_id = '`+ws+`' THEN RAISE EXCEPTION 'fixture rejection'; END IF; RETURN NEW; END; $$; CREATE TRIGGER fixture_rejection BEFORE INSERT ON start_outbox FOR EACH ROW EXECUTE FUNCTION reject_fixture_outbox()`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, e := s.Pool.Exec(ctx, "DROP TRIGGER fixture_rejection ON start_outbox; DROP FUNCTION reject_fixture_outbox()"); e != nil {
			t.Error(e)
		}
	})
	if _, err = s.Admit(ctx, ws, w.ID, "event_1", []byte(`{}`)); err == nil {
		t.Fatal("fault not injected")
	}
	var n int
	if err = s.Pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM runs WHERE workspace_id=$1)+(SELECT count(*) FROM trigger_inbox WHERE workspace_id=$1)", ws).Scan(&n); err != nil || n != 0 {
		t.Fatal("partial admission persisted", n, err)
	}
}
func TestAdmissionRequiresPublication(t *testing.T) {
	s := database(t)
	ws := workspace(t, s)
	w := create(t, s, ws)
	if _, err := s.Admit(context.Background(), ws, w.ID, "event_1", []byte(`{}`)); !errors.Is(err, ErrUnpublished) {
		t.Fatal(err)
	}
}
