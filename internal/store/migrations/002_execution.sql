ALTER TABLE runs DROP CONSTRAINT runs_state_check;
ALTER TABLE runs ADD CONSTRAINT runs_state_check CHECK (state IN ('accepted','running','waiting','needs_attention','succeeded','failed','cancelled'));
ALTER TABLE runs ADD COLUMN execution_mode text NOT NULL DEFAULT 'admission' CHECK (execution_mode IN ('admission','execute'));
ALTER TABLE runs ADD COLUMN cancel_requested boolean NOT NULL DEFAULT false;
ALTER TABLE runs ADD COLUMN checkpoint bigint NOT NULL DEFAULT 0;
ALTER TABLE runs ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
CREATE TABLE payloads (
 workspace_id text NOT NULL,
 run_id text NOT NULL,
 id text NOT NULL,
 data jsonb NOT NULL,
 digest text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(workspace_id,run_id,id),
 FOREIGN KEY(workspace_id,run_id) REFERENCES runs(workspace_id,id)
);
CREATE TABLE node_invocations (
 workspace_id text NOT NULL,
 run_id text NOT NULL,
 id text NOT NULL,
 node_id text NOT NULL,
 envelope jsonb NOT NULL,
 accepted_attempt integer,
 accepted_output_ref text,
 PRIMARY KEY(workspace_id,run_id,id),
 UNIQUE(workspace_id,run_id,node_id),
 FOREIGN KEY(workspace_id,run_id) REFERENCES runs(workspace_id,id),
 FOREIGN KEY(workspace_id,run_id,accepted_output_ref) REFERENCES payloads(workspace_id,run_id,id)
);
CREATE TABLE attempts (
 workspace_id text NOT NULL,
 run_id text NOT NULL,
 invocation_id text NOT NULL,
 attempt integer NOT NULL CHECK(attempt>0),
 status text NOT NULL CHECK(status IN ('succeeded','failed','uncertain')),
 output_ref text,
 error_code text,
 completed_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(workspace_id,run_id,invocation_id,attempt),
 FOREIGN KEY(workspace_id,run_id,invocation_id) REFERENCES node_invocations(workspace_id,run_id,id),
 FOREIGN KEY(workspace_id,run_id,output_ref) REFERENCES payloads(workspace_id,run_id,id)
);
CREATE TRIGGER no_attempt_change BEFORE UPDATE OR DELETE ON attempts FOR EACH ROW EXECUTE FUNCTION immutable_version();
CREATE TABLE run_events (
 workspace_id text NOT NULL,
 run_id text NOT NULL,
 sequence bigint NOT NULL CHECK(sequence>0),
 event jsonb NOT NULL,
 recorded_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(workspace_id,run_id,sequence),
 FOREIGN KEY(workspace_id,run_id) REFERENCES runs(workspace_id,id)
);
CREATE TRIGGER no_event_change BEFORE UPDATE OR DELETE ON run_events FOR EACH ROW EXECUTE FUNCTION immutable_version();
