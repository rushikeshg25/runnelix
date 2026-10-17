package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rushikeshg25/runnelix/internal/definition"
	"time"
)

var ErrOverlap = errors.New("workflow already active")

type ScheduleSpec struct {
	Kind            string          `json:"kind"`
	IntervalSeconds int             `json:"intervalSeconds"`
	Hour            int             `json:"hour"`
	Minute          int             `json:"minute"`
	Timezone        string          `json:"timezone"`
	Enabled         bool            `json:"enabled"`
	Input           json.RawMessage `json:"input"`
}
type Schedule struct {
	WorkspaceID      string       `json:"workspaceId"`
	ID               string       `json:"id"`
	WorkflowID       string       `json:"workflowId"`
	Revision         int64        `json:"revision"`
	ObservedRevision int64        `json:"observedRevision"`
	Spec             ScheduleSpec `json:"spec"`
}
type Occurrence struct {
	ScheduleID string `json:"scheduleId"`
	Key        string `json:"key"`
	RunID      string `json:"runId"`
	Outcome    string `json:"outcome"`
}

func validateSchedule(spec ScheduleSpec) error {
	if spec.Kind != "interval" && spec.Kind != "daily" {
		return ErrInvalid
	}
	if spec.Kind == "interval" && (spec.IntervalSeconds < 60 || spec.IntervalSeconds > 86400 || spec.Timezone != "UTC") {
		return ErrInvalid
	}
	if spec.Kind == "daily" && (spec.Hour < 0 || spec.Hour > 23 || spec.Minute < 0 || spec.Minute > 59) {
		return ErrInvalid
	}
	if _, err := time.LoadLocation(spec.Timezone); err != nil {
		return ErrInvalid
	}
	var input any
	if len(spec.Input) > 8192 || definition.Decode(spec.Input, &input) != nil {
		return ErrInvalid
	}
	if _, ok := input.(map[string]any); !ok {
		return ErrInvalid
	}
	return nil
}
func (s *Store) SaveSchedule(ctx context.Context, schedule Schedule, expected int64) (Schedule, error) {
	if err := validateSchedule(schedule.Spec); err != nil {
		return schedule, err
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return schedule, err
	}
	defer tx.Rollback(ctx)
	var ws string
	if err = tx.QueryRow(ctx, "SELECT id FROM workspaces WHERE id=$1 FOR UPDATE", schedule.WorkspaceID).Scan(&ws); err != nil {
		return schedule, notFound(err)
	}
	raw, _ := json.Marshal(schedule.Spec)
	if expected == 0 {
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM schedules WHERE workspace_id=$1", schedule.WorkspaceID).Scan(&count); err != nil {
			return schedule, err
		}
		if count >= 20 {
			return schedule, ErrLimit
		}
		schedule.ID = "schedule_" + uuid.NewString()
		schedule.Revision = 1
		_, err = tx.Exec(ctx, "INSERT INTO schedules(workspace_id,id,workflow_id,spec) VALUES($1,$2,$3,$4)", schedule.WorkspaceID, schedule.ID, schedule.WorkflowID, raw)
	} else {
		tag, e := tx.Exec(ctx, "UPDATE schedules SET spec=$4,revision=revision+1 WHERE workspace_id=$1 AND id=$2 AND revision=$3 AND workflow_id=$5", schedule.WorkspaceID, schedule.ID, expected, raw, schedule.WorkflowID)
		err = e
		if err == nil && tag.RowsAffected() != 1 {
			return schedule, ErrConflict
		}
		schedule.Revision = expected + 1
	}
	if err != nil {
		return schedule, err
	}
	return schedule, tx.Commit(ctx)
}
func scanSchedule(row pgx.Row) (Schedule, error) {
	var s Schedule
	var raw []byte
	err := row.Scan(&s.WorkspaceID, &s.ID, &s.WorkflowID, &s.Revision, &s.ObservedRevision, &raw)
	if err != nil {
		return s, notFound(err)
	}
	err = json.Unmarshal(raw, &s.Spec)
	return s, err
}
func (s *Store) Schedules(ctx context.Context, workspace string) ([]Schedule, error) {
	rows, err := s.Pool.Query(ctx, "SELECT workspace_id,id,workflow_id,revision,observed_revision,spec FROM schedules WHERE workspace_id=$1 ORDER BY id LIMIT 20", workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Schedule{}
	for rows.Next() {
		item, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// Serialize desired-state publication per schedule across dispatcher replicas. The
// bounded external call runs under this lock, preventing older revisions winning last.
func (s *Store) SyncSchedule(ctx context.Context, apply func(Schedule) error) (bool, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	item, err := scanSchedule(tx.QueryRow(ctx, "SELECT workspace_id,id,workflow_id,revision,observed_revision,spec FROM schedules WHERE revision<>observed_revision ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 1"))
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = apply(item); err != nil {
		return true, err
	}
	_, err = tx.Exec(ctx, "UPDATE schedules SET observed_revision=revision WHERE workspace_id=$1 AND id=$2", item.WorkspaceID, item.ID)
	if err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}
func occurrenceKey(s Schedule, at time.Time) string {
	identity := at.UTC().Format(time.RFC3339Nano)
	if s.Spec.Kind == "daily" {
		location, _ := time.LoadLocation(s.Spec.Timezone)
		identity = at.In(location).Format("2006-01-02T15:04")
	}
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%s", s.ID, s.Revision, identity)))
	return "occurrence_" + hex.EncodeToString(sum[:])
}
func (s *Store) AdmitSchedule(ctx context.Context, workspace, id string, revision int64, at time.Time) (Occurrence, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Occurrence{}, err
	}
	defer tx.Rollback(ctx)
	var ws string
	if err = tx.QueryRow(ctx, "SELECT id FROM workspaces WHERE id=$1 FOR UPDATE", workspace).Scan(&ws); err != nil {
		return Occurrence{}, notFound(err)
	}
	schedule, err := scanSchedule(tx.QueryRow(ctx, "SELECT workspace_id,id,workflow_id,revision,observed_revision,spec FROM schedules WHERE workspace_id=$1 AND id=$2 FOR UPDATE", workspace, id))
	if err != nil {
		return Occurrence{}, err
	}
	pinned := schedule
	pinned.Revision = revision
	key := occurrenceKey(pinned, at)
	result := Occurrence{ScheduleID: id, Key: key}
	err = tx.QueryRow(ctx, "SELECT coalesce(run_id,''),outcome FROM schedule_occurrences WHERE workspace_id=$1 AND schedule_id=$2 AND occurrence_key=$3", workspace, id, key).Scan(&result.RunID, &result.Outcome)
	if err == nil {
		return result, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, err
	}
	result.Outcome = "accepted"
	if !schedule.Spec.Enabled {
		result.Outcome = "disabled"
	} else if revision != schedule.Revision {
		result.Outcome = "stale"
	} else {
		var value any
		if definition.Decode(schedule.Spec.Input, &value) != nil {
			return result, ErrInvalid
		}
		raw, _ := json.Marshal(value)
		sum := sha256.Sum256(raw)
		run, e := s.admitTx(ctx, tx, workspace, schedule.WorkflowID, key, raw, hex.EncodeToString(sum[:]), "execute", "schedule")
		if errors.Is(e, ErrOverlap) {
			result.Outcome = "overlap"
		} else if errors.Is(e, ErrLimit) {
			result.Outcome = "limit"
		} else if e != nil {
			return result, e
		} else {
			result.RunID = run.ID
		}
	}
	_, err = tx.Exec(ctx, "INSERT INTO schedule_occurrences(workspace_id,schedule_id,occurrence_key,schedule_revision,scheduled_at,run_id,outcome) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7)", workspace, id, key, revision, at, result.RunID, result.Outcome)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
