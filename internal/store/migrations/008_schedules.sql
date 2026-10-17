ALTER TABLE trigger_inbox DROP CONSTRAINT trigger_inbox_source_check;
ALTER TABLE trigger_inbox ADD CONSTRAINT trigger_inbox_source_check CHECK(source IN ('manual','webhook','schedule'));
CREATE TABLE schedules (
 workspace_id text NOT NULL,
 id text NOT NULL,
 workflow_id text NOT NULL,
 revision bigint NOT NULL DEFAULT 1,
 observed_revision bigint NOT NULL DEFAULT 0,
 spec jsonb NOT NULL,
 PRIMARY KEY(workspace_id,id),
 FOREIGN KEY(workspace_id,workflow_id) REFERENCES workflows(workspace_id,id)
);
CREATE TABLE schedule_occurrences (
 workspace_id text NOT NULL,
 schedule_id text NOT NULL,
 occurrence_key text NOT NULL,
 schedule_revision bigint NOT NULL,
 scheduled_at timestamptz NOT NULL,
 run_id text,
 outcome text NOT NULL CHECK(outcome IN ('accepted','overlap','disabled','stale','limit')),
 PRIMARY KEY(workspace_id,schedule_id,occurrence_key),
 FOREIGN KEY(workspace_id,schedule_id) REFERENCES schedules(workspace_id,id),
 FOREIGN KEY(workspace_id,run_id) REFERENCES runs(workspace_id,id)
);
CREATE TRIGGER no_occurrence_change BEFORE UPDATE OR DELETE ON schedule_occurrences FOR EACH ROW EXECUTE FUNCTION immutable_version();
