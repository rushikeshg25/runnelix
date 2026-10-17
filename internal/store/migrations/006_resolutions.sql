CREATE TABLE uncertain_invocations (
 workspace_id text NOT NULL,
 run_id text NOT NULL,
 invocation_id text NOT NULL,
 resolved_at timestamptz,
 PRIMARY KEY(workspace_id,run_id,invocation_id),
 FOREIGN KEY(workspace_id,run_id,invocation_id) REFERENCES node_invocations(workspace_id,run_id,id)
);
CREATE TABLE operator_resolutions (
 workspace_id text NOT NULL,
 run_id text NOT NULL,
 invocation_id text NOT NULL,
 action text NOT NULL CHECK(action IN ('confirm','fail')),
 output_ref text,
 actor text NOT NULL,
 note text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(workspace_id,run_id,invocation_id),
 FOREIGN KEY(workspace_id,run_id,invocation_id) REFERENCES uncertain_invocations(workspace_id,run_id,invocation_id),
 FOREIGN KEY(workspace_id,run_id,output_ref) REFERENCES payloads(workspace_id,run_id,id)
);
CREATE TRIGGER no_resolution_change BEFORE UPDATE OR DELETE ON operator_resolutions FOR EACH ROW EXECUTE FUNCTION immutable_version();
