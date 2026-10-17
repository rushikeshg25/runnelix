package engine

import (
	"context"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/store"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"strings"
	"testing"
	"time"
)

func TestV3DurableWaitAndMergeSkip(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	r := store.StartRequest{WorkspaceID: "workspace_test", RunID: "run_test", ExecutionMode: "execute", InterpreterVersion: 3}
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: store.RuntimeIDFor(r)})
	step := func(id, typ string, config map[string]any) definition.Step {
		return definition.Step{Kind: "control", RetrySafety: "pure", Node: definition.Node{ID: id, Type: typ, Version: 1, Config: config}, ImplementationDigest: "sha256:" + strings.Repeat("a", 64)}
	}
	start := step("start", "trigger.manual", map[string]any{})
	start.Kind = "trigger"
	plan := definition.Plan{InterpreterVersion: 3, Steps: []definition.Step{start, step("condition", "control.condition", map[string]any{}), step("merge", "control.merge", map[string]any{"policy": "named"}), step("wait", "control.wait", map[string]any{"seconds": float64(3600)})}, Edges: []definition.Edge{
		{From: definition.Endpoint{Node: "start", Port: "out"}, To: definition.Endpoint{Node: "condition", Port: "in"}},
		{From: definition.Endpoint{Node: "condition", Port: "true"}, To: definition.Endpoint{Node: "merge", Port: "a"}},
		{From: definition.Endpoint{Node: "condition", Port: "false"}, To: definition.Endpoint{Node: "merge", Port: "b"}},
		{From: definition.Endpoint{Node: "merge", Port: "out"}, To: definition.Endpoint{Node: "wait", Port: "in"}},
	}}
	events := []store.Event{}
	env.RegisterActivityWithOptions(func(context.Context, store.StartRequest) (store.Snapshot, error) {
		return store.Snapshot{Plan: plan, InputRef: "payload_in"}, nil
	}, activity.RegisterOptions{Name: "Bootstrap"})
	env.RegisterActivityWithOptions(func(_ context.Context, p Projection) error { events = append(events, p.Event); return nil }, activity.RegisterOptions{Name: "Project"})
	env.RegisterActivityWithOptions(func(context.Context, StepRequest) (ControlResult, error) {
		return ControlResult{OutputRef: "payload_in", Port: "true"}, nil
	}, activity.RegisterOptions{Name: "Control"})
	env.RegisterActivityWithOptions(func(_ context.Context, m MergeRequest) (string, error) {
		if len(m.Inputs) != 1 || m.Inputs["a"] != "payload_in" {
			t.Error("merge included skipped branch")
		}
		return "payload_merge", nil
	}, activity.RegisterOptions{Name: "Merge"})
	before := env.Now()
	env.ExecuteWorkflow(ExecuteV3, r)
	if err := env.GetWorkflowError(); err != nil {
		t.Fatal(err)
	}
	if env.Now().Sub(before) < time.Hour {
		t.Fatal("wait did not use durable clock")
	}
	waiting := false
	for _, e := range events {
		if e.State == "waiting" {
			waiting = true
		}
	}
	if !waiting || events[len(events)-1].State != "succeeded" {
		t.Fatal(events)
	}
}
