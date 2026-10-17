package dispatch

import (
	"context"
	"errors"
	"github.com/rushikeshg25/runnelix/internal/engine"
	"github.com/rushikeshg25/runnelix/internal/store"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"time"
)

// RepairProjection copies runtime-owned facts. It never schedules workflow steps.
func RepairProjection(ctx context.Context, s *store.Store, c client.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	r, checkpoint, err := s.ProjectionCandidate(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	description, err := c.DescribeWorkflowExecution(ctx, store.RuntimeIDFor(r), "")
	if err != nil {
		return err
	}
	var events []store.Event
	if description.WorkflowExecutionInfo.Status == enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED {
		var result engine.ExecutionResult
		if err = c.GetWorkflow(ctx, store.RuntimeIDFor(r), "").Get(ctx, &result); err != nil {
			return err
		}
		for _, event := range result.Events {
			if event.Sequence > checkpoint {
				events = append(events, event)
			}
		}
	} else {
		value, err := c.QueryWorkflow(ctx, store.RuntimeIDFor(r), "", "ProjectionJournal", checkpoint)
		if err != nil {
			return err
		}
		if err = value.Get(&events); err != nil {
			return err
		}
	}
	for _, event := range events {
		if err = s.ApplyEvent(ctx, r, event); err != nil {
			return err
		}
	}
	return nil
}
