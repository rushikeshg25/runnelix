package store

import (
	"context"
	"encoding/json"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/secrets"
	"strings"
	"testing"
)

func TestPortableImportAtomicScopedAndUnpublished(t *testing.T) {
	s := database(t)
	ctx := context.Background()
	ws, w, _ := published(t, s)
	target := workspace(t, s)
	var g definition.Graph
	if err := json.Unmarshal(w.Graph, &g); err != nil {
		t.Fatal(err)
	}
	g.Nodes[1] = definition.Node{ID: g.Nodes[1].ID, Type: "slack.postMessage", Version: 1, Config: map[string]any{"channel": "C123", "credentialRef": "credential_source"}}
	data, _ := json.Marshal(g)
	bundle, err := definition.ExportPortable("Imported", data, s.Catalog)
	if err != nil {
		t.Fatal(err)
	}
	box, _ := secrets.New(strings.Repeat("ab", 32), "fixture")
	foreign, err := s.CreateCredential(ctx, ws, "Foreign", "bearer", "fake", "slack.com", box)
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := s.CreateCredential(ctx, target, "Wrong destination", "bearer", "fake", "api.github.com", box)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{foreign.ID, wrong.ID, "credential_missing"} {
		if _, err = s.ImportWorkflow(ctx, target, bundle, map[string]string{"slot_1": id}); err == nil {
			t.Fatal("bad binding accepted")
		}
	}
	var count int
	if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM workflows WHERE workspace_id=$1", target).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed import left workflows", err)
	}
	c, err := s.CreateCredential(ctx, target, "Local", "bearer", "fake", "slack.com", box)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := s.ImportWorkflow(ctx, target, bundle, map[string]string{"slot_1": c.ID})
	if err != nil {
		t.Fatal(err)
	}
	if imported.ActiveVersionID != nil || imported.Revision != 1 || imported.ID == w.ID {
		t.Fatal("import reused publication or identity")
	}
	if err = s.CredentialAllowed(ctx, target, imported.ID, c.ID, "bearer"); err != nil {
		t.Fatal(err)
	}
	importedAudit, grantAudit := false, false
	for _, record := range auditRows(t, s, target) {
		if record.Action == "workflow.imported" && record.TargetID == imported.ID {
			importedAudit = true
		}
		if record.Action == "credential.granted" && record.TargetID == c.ID && record.Details.WorkflowID == imported.ID {
			grantAudit = true
		}
	}
	if !importedAudit || !grantAudit {
		t.Fatal("bound import missing audit records")
	}
	if _, err = s.Publish(ctx, target, imported.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err = s.RevokeCredential(ctx, target, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ImportWorkflow(ctx, target, bundle, map[string]string{"slot_1": c.ID}); err == nil {
		t.Fatal("revoked binding accepted")
	}
}
