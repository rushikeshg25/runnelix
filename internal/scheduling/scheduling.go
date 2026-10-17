// Package scheduling delegates all clocks to Temporal. PostgreSQL owns admission.
package scheduling

import (
	"context"
	"errors"
	"fmt"
	"github.com/rushikeshg25/runnelix/internal/store"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
	"time"
)

const Queue = "runnelix-schedule-admission-v1"
const WorkflowName = "RunnelixScheduleAdmissionV1"

type Delivery struct {
	WorkspaceID string    `json:"workspaceId"`
	ScheduleID  string    `json:"scheduleId"`
	Revision    int64     `json:"revision"`
	At          time.Time `json:"at"`
}

func Admission(ctx workflow.Context, d Delivery) (store.Occurrence, error) {
	at, ok := workflow.GetTypedSearchAttributes(ctx).GetTime(temporal.NewSearchAttributeKeyTime("TemporalScheduledStartTime"))
	if !ok {
		return store.Occurrence{}, temporal.NewNonRetryableApplicationError("Scheduled time is missing", "InvalidSchedule", nil)
	}
	d.At = at
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 10 * time.Second, ScheduleToCloseTimeout: time.Minute, RetryPolicy: &temporal.RetryPolicy{MaximumAttempts: 5}})
	var result store.Occurrence
	err := workflow.ExecuteActivity(ctx, "AdmitScheduleOccurrence", d).Get(ctx, &result)
	return result, err
}
func Activity(s *store.Store) func(context.Context, Delivery) (store.Occurrence, error) {
	return func(ctx context.Context, d Delivery) (store.Occurrence, error) {
		r, err := s.AdmitSchedule(ctx, d.WorkspaceID, d.ScheduleID, d.Revision, d.At)
		if err != nil {
			return r, errors.New("schedule admission unavailable")
		}
		return r, nil
	}
}
func RuntimeID(s store.Schedule) string {
	return fmt.Sprintf("runnelix-schedule-%s-%s", s.WorkspaceID, s.ID)
}
func Spec(s store.ScheduleSpec) client.ScheduleSpec {
	spec := client.ScheduleSpec{TimeZoneName: s.Timezone}
	if s.Kind == "interval" {
		spec.Intervals = []client.ScheduleIntervalSpec{{Every: time.Duration(s.IntervalSeconds) * time.Second}}
	} else {
		spec.Calendars = []client.ScheduleCalendarSpec{{Hour: []client.ScheduleRange{{Start: s.Hour}}, Minute: []client.ScheduleRange{{Start: s.Minute}}, Second: []client.ScheduleRange{{Start: 0}}}}
	}
	return spec
}
func Reconcile(ctx context.Context, s *store.Store, c client.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.SyncSchedule(ctx, func(item store.Schedule) error {
		spec := Spec(item.Spec)
		action := &client.ScheduleWorkflowAction{ID: RuntimeID(item) + "-occurrence", Workflow: WorkflowName, Args: []any{Delivery{WorkspaceID: item.WorkspaceID, ScheduleID: item.ID, Revision: item.Revision}}, TaskQueue: Queue, WorkflowExecutionTimeout: 2 * time.Minute}
		policy := client.SchedulePolicies{Overlap: enumspb.SCHEDULE_OVERLAP_POLICY_SKIP, CatchupWindow: time.Minute, PauseOnFailure: true}
		state := client.ScheduleState{Paused: !item.Spec.Enabled, Note: "Managed by Runnelix desired state"}
		_, e := c.ScheduleClient().Create(ctx, client.ScheduleOptions{ID: RuntimeID(item), Spec: spec, Action: action, Overlap: policy.Overlap, CatchupWindow: policy.CatchupWindow, PauseOnFailure: true, Paused: state.Paused, Note: state.Note})
		if !errors.Is(e, temporal.ErrScheduleAlreadyRunning) {
			return e
		}
		return c.ScheduleClient().GetHandle(ctx, RuntimeID(item)).Update(ctx, client.ScheduleUpdateOptions{DoUpdate: func(client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
			return &client.ScheduleUpdate{Schedule: &client.Schedule{Spec: &spec, Action: action, Policy: &policy, State: &state}}, nil
		}})
	})
	return err
}
