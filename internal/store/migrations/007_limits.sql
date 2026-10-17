ALTER TABLE runs ADD COLUMN max_invocations integer NOT NULL DEFAULT 1000 CHECK(max_invocations BETWEEN 1 AND 1000);
ALTER TABLE runs ADD COLUMN invocation_count integer NOT NULL DEFAULT 0;
ALTER TABLE runs ADD COLUMN max_payload_bytes bigint NOT NULL DEFAULT 8388608 CHECK(max_payload_bytes BETWEEN 8192 AND 8388608);
ALTER TABLE runs ADD COLUMN payload_bytes bigint NOT NULL DEFAULT 0;
UPDATE runs r SET invocation_count=(SELECT count(*) FROM node_invocations i WHERE (i.workspace_id,i.run_id)=(r.workspace_id,r.id)),payload_bytes=(SELECT coalesce(sum(octet_length(p.data::text)),0) FROM payloads p WHERE (p.workspace_id,p.run_id)=(r.workspace_id,r.id));
CREATE TABLE provider_limits (
 workspace_id text NOT NULL REFERENCES workspaces(id),
 bucket text NOT NULL,
 next_allowed_at timestamptz NOT NULL,
 PRIMARY KEY(workspace_id,bucket)
);
