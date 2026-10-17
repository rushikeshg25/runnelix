package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/rushikeshg25/runnelix/internal/definition"
	"github.com/rushikeshg25/runnelix/internal/store"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

func TestBranchSelectionSkipAndFailure(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failure], func(t *testing.T) {
			var suite testsuite.WorkflowTestSuite
			env := suite.NewTestWorkflowEnvironment()
			r := store.StartRequest{WorkspaceID: "workspace_test", RunID: "run_test", ExecutionMode: "execute", VersionID: "version_test", PlanDigest: strings.Repeat("a", 64)}
			env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: store.RuntimeIDFor(r)})
			step := func(id, kind, typ string) definition.Step {
				return definition.Step{Kind: kind, RetrySafety: "pure", Node: definition.Node{ID: id, Type: typ, Version: 1, Config: map[string]any{}}, ImplementationDigest: "sha256:" + strings.Repeat("a", 64)}
			}
			plan := definition.Plan{FormatVersion: 1, InterpreterVersion: 2, Steps: []definition.Step{step("start", "trigger", "trigger.manual"), step("choice", "control", "control.condition"), step("left", "action", "mock.echo"), step("right", "action", "mock.echo")}, Edges: []definition.Edge{{From: definition.Endpoint{Node: "start", Port: "out"}, To: definition.Endpoint{Node: "choice", Port: "in"}}, {From: definition.Endpoint{Node: "choice", Port: "true"}, To: definition.Endpoint{Node: "left", Port: "in"}}, {From: definition.Endpoint{Node: "choice", Port: "false"}, To: definition.Endpoint{Node: "right", Port: "in"}}}}
			events := []store.Event{}
			env.RegisterActivityWithOptions(func(context.Context, store.StartRequest) (store.Snapshot, error) {
				return store.Snapshot{Plan: plan, InputRef: "input"}, nil
			}, activity.RegisterOptions{Name: "Bootstrap"})
			env.RegisterActivityWithOptions(func(_ context.Context, p Projection) error { events = append(events, p.Event); return nil }, activity.RegisterOptions{Name: "Project"})
			env.RegisterActivityWithOptions(func(context.Context, StepRequest) (ControlResult, error) {
				return ControlResult{OutputRef: "input", Port: "true"}, nil
			}, activity.RegisterOptions{Name: "Control"})
			env.RegisterActivityWithOptions(func(_ context.Context, s StepRequest) (definition.Envelope, error) {
				if s.Step.ID != "left" {
					t.Error("unselected action prepared")
				}
				return definition.Envelope{NodeID: s.Step.ID}, nil
			}, activity.RegisterOptions{Name: "Prepare"})
			env.RegisterActivityWithOptions(func(context.Context, definition.Envelope) (Completion, error) {
				if failure {
					return Completion{}, temporal.NewNonRetryableApplicationError("safe failure", "ProviderRejected", nil)
				}
				return Completion{OutputRef: "result", Attempt: 1}, nil
			}, activity.RegisterOptions{Name: NodeActivity})
			env.RegisterActivityWithOptions(func(context.Context, Selection) (string, error) { return "result", nil }, activity.RegisterOptions{Name: "Select"})
			env.ExecuteWorkflow(Execute, r)
			if (env.GetWorkflowError() != nil) != failure {
				t.Fatal(env.GetWorkflowError())
			}
			for i, e := range events {
				if e.Sequence != int64(i+1) {
					t.Fatal("event sequence gap")
				}
			}
			if failure {
				if events[len(events)-1].State != "failed" {
					t.Fatal("failure not projected")
				}
			} else {
				if events[len(events)-1].State != "succeeded" {
					t.Fatal("success not projected")
				}
				skipped := false
				for _, e := range events {
					if e.NodeID == "right" && e.NodeState == "skipped" {
						skipped = true
					}
				}
				if !skipped {
					t.Fatal("skip not recorded")
				}
			}
		})
	}
}
func TestPreExecutionCancellation(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	r := store.StartRequest{WorkspaceID: "w", RunID: "run_test", ExecutionMode: "execute"}
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: store.RuntimeIDFor(r)})
	env.RegisterActivityWithOptions(func(context.Context, store.StartRequest) (store.Snapshot, error) {
		return store.Snapshot{Plan: definition.Plan{InterpreterVersion: 2}, CancelRequested: true}, nil
	}, activity.RegisterOptions{Name: "Bootstrap"})
	env.RegisterActivityWithOptions(func(context.Context, Projection) error { return nil }, activity.RegisterOptions{Name: "Project"})
	env.OnActivity("Project", mock.Anything, mock.MatchedBy(func(p Projection) bool { return p.Event.State == "cancelled" })).Return(nil).Once()
	env.ExecuteWorkflow(Execute, r)
	if !temporal.IsCanceledError(env.GetWorkflowError()) {
		t.Fatal(env.GetWorkflowError())
	}
	env.AssertExpectations(t)
}
