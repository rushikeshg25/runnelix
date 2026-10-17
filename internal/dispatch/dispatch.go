// Package dispatch bridges committed admission to Temporal; it never interprets a graph.
package dispatch

import (
	"context"
	"errors"
	"time"

	"github.com/rushikeshg25/runnelix/internal/engine"
	"github.com/rushikeshg25/runnelix/internal/store"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const WorkflowName = "RunnelixAdmissionV1"
const Queue = "runnelix-admission-v1"
const ReceiptActivity = "RecordAdmissionReceiptV1"

type Starter interface {
	Start(context.Context, store.StartRequest) error
}
type TemporalStarter struct{ Client client.Client }

func (t TemporalStarter) Start(ctx context.Context, r store.StartRequest) error {
	name, queue := WorkflowName, Queue
	if r.ExecutionMode == "execute" {
		name, queue = engine.WorkflowName, engine.Queue
	}
	_, err := t.Client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{ID: store.RuntimeIDFor(r), TaskQueue: queue, WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE, WorkflowIDConflictPolicy: enumspb.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING, WorkflowExecutionTimeout: 24 * time.Hour}, name, r)
	var duplicate *serviceerror.WorkflowExecutionAlreadyStarted
	if errors.As(err, &duplicate) {
		return nil
	}
	return err
}

// Once sends at most one request. It acknowledges only a durable database receipt,
// never merely a successful StartWorkflow RPC. Ambiguous responses are retried.
func Once(ctx context.Context, s *store.Store, starter Starter) (bool, error) {
	c, err := s.Claim(ctx, 10*time.Second)
	if store.IsEmpty(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	receipt, err := s.HasReceipt(ctx, c.StartRequest)
	if err != nil {
		return true, err
	}
	if receipt {
		return true, s.Acknowledge(ctx, c)
	}
	sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sendErr := starter.Start(sendCtx, c.StartRequest)
	delay := time.Second
	if sendErr != nil {
		delay = time.Duration(min(c.Attempts, 60)) * time.Second
	}
	if err = s.Retry(ctx, c, delay); err != nil {
		return true, err
	}
	return true, sendErr
}

// Admit is a receipt-only workflow. A completed receipt does not complete the product run.
func Admit(ctx workflow.Context, r store.StartRequest) error {
	if r.RunID == "" || r.WorkspaceID == "" || r.VersionID == "" || len(r.PlanDigest) != 64 || workflow.GetInfo(ctx).WorkflowExecution.ID != store.RuntimeID(r.RunID) {
		return temporal.NewNonRetryableApplicationError("Invalid admission identity", "InvalidAdmission", nil)
	}
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 10 * time.Second, ScheduleToCloseTimeout: 23 * time.Hour, RetryPolicy: &temporal.RetryPolicy{InitialInterval: time.Second, MaximumInterval: time.Minute}})
	return workflow.ExecuteActivity(ctx, ReceiptActivity, r).Get(ctx, nil)
}
func Receipt(s *store.Store) func(context.Context, store.StartRequest) error {
	return func(ctx context.Context, r store.StartRequest) error {
		err := s.RecordReceipt(ctx, r, activity.GetInfo(ctx).WorkflowExecution.ID)
		if errors.Is(err, store.ErrConflict) {
			return temporal.NewNonRetryableApplicationError("Admission identity mismatch", "InvalidAdmission", nil)
		}
		if err != nil {
			return errors.New("admission receipt storage unavailable")
		}
		return nil
	}
}

// ReconcileCancellation repeats cancellation intent until a terminal projection is durable.
func ReconcileCancellation(ctx context.Context, s *store.Store, c client.Client) error {
	pending, err := s.PendingCancellations(ctx)
	if err != nil {
		return err
	}
	for _, r := range pending {
		if err = c.CancelWorkflow(ctx, store.RuntimeIDFor(r), ""); err != nil {
			var absent *serviceerror.NotFound
			if !errors.As(err, &absent) {
				return err
			}
		}
	}
	return nil
}
