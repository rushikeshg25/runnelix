package engine

import (
	"context"
	"fmt"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/store"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	"strconv"
	"strings"
	"sync/atomic"
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

func TestV3MapOrderingEmptyAndBounds(t *testing.T) {
	for _, size := range []int{0, 5} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var suite testsuite.WorkflowTestSuite
			env := suite.NewTestWorkflowEnvironment()
			r := store.StartRequest{WorkspaceID: "w", RunID: "run_map", ExecutionMode: "execute", InterpreterVersion: 3}
			env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: store.RuntimeIDFor(r)})
			body := definition.Step{Node: definition.Node{ID: "map", Type: "mock.echo", Version: 1, Config: map[string]any{}}, Kind: "action", RetrySafety: "pure", ImplementationDigest: "sha256:" + strings.Repeat("a", 64)}
			plan := definition.Plan{InterpreterVersion: 3, Steps: []definition.Step{{Node: definition.Node{ID: "start"}, Kind: "trigger"}, {Node: definition.Node{ID: "map", Type: "control.map", Config: map[string]any{"concurrency": float64(2)}}, Kind: "control", Body: &body}}, Edges: []definition.Edge{{From: definition.Endpoint{Node: "start", Port: "out"}, To: definition.Endpoint{Node: "map", Port: "in"}}}}
			var active, maxActive atomic.Int32
			var sequence int64
			env.RegisterActivityWithOptions(func(context.Context, store.StartRequest) (store.Snapshot, error) {
				return store.Snapshot{Plan: plan, InputRef: "input"}, nil
			}, activity.RegisterOptions{Name: "Bootstrap"})
			env.RegisterActivityWithOptions(func(_ context.Context, p Projection) error {
				if p.Event.Sequence != sequence+1 {
					t.Error("projection order")
				}
				sequence = p.Event.Sequence
				return nil
			}, activity.RegisterOptions{Name: "Project"})
			env.RegisterActivityWithOptions(func(context.Context, StepRequest) ([]string, error) {
				refs := []string{}
				for i := 0; i < size; i++ {
					refs = append(refs, fmt.Sprint(i))
				}
				return refs, nil
			}, activity.RegisterOptions{Name: "Expand"})
			env.RegisterActivityWithOptions(func(_ context.Context, s ScopedRequest) (definition.Envelope, error) {
				return definition.Envelope{InputRef: s.InputRef, InvocationID: s.Scope}, nil
			}, activity.RegisterOptions{Name: "PrepareScoped"})
			env.RegisterActivityWithOptions(func(_ context.Context, e definition.Envelope) (Completion, error) {
				n := active.Add(1)
				for {
					old := maxActive.Load()
					if n <= old || maxActive.CompareAndSwap(old, n) {
						break
					}
				}
				defer active.Add(-1)
				i, _ := strconv.Atoi(e.InputRef)
				time.Sleep(time.Duration(6-i) * time.Millisecond)
				return Completion{OutputRef: e.InputRef, Attempt: 1}, nil
			}, activity.RegisterOptions{Name: NodeActivity})
			env.RegisterActivityWithOptions(func(_ context.Context, s Selection) (string, error) { return s.Completion.OutputRef, nil }, activity.RegisterOptions{Name: "Select"})
			env.RegisterActivityWithOptions(func(_ context.Context, a AssembleRequest) (string, error) {
				if len(a.Refs) != size {
					t.Error("missing results")
				}
				for i, ref := range a.Refs {
					if ref != fmt.Sprint(i) {
						t.Error("out of order")
					}
				}
				return "assembled", nil
			}, activity.RegisterOptions{Name: "Assemble"})
			env.ExecuteWorkflow(ExecuteV3, r)
			if err := env.GetWorkflowError(); err != nil {
				t.Fatal(err)
			}
			if maxActive.Load() > 2 {
				t.Fatal("concurrency bound exceeded")
			}
			if size == 0 && maxActive.Load() != 0 {
				t.Fatal("empty map executed actions")
			}
		})
	}
}
