CREATE TABLE workspaces (
 id text PRIMARY KEY,
 name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120)
);
CREATE TABLE workflows (
 workspace_id text NOT NULL REFERENCES workspaces(id),
 id text NOT NULL,
 name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
 active_version_id text,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (workspace_id,id)
);
CREATE TABLE workflow_drafts (
 workspace_id text NOT NULL,
 workflow_id text NOT NULL,
 revision bigint NOT NULL DEFAULT 1 CHECK (revision BETWEEN 1 AND 9007199254740991),
 graph jsonb NOT NULL,
 PRIMARY KEY (workspace_id,workflow_id),
 FOREIGN KEY (workspace_id,workflow_id) REFERENCES workflows(workspace_id,id)
);
CREATE TABLE workflow_versions (
 workspace_id text NOT NULL,
 workflow_id text NOT NULL,
 id text NOT NULL,
 sequence bigint NOT NULL CHECK (sequence > 0),
 draft_revision bigint NOT NULL,
 graph jsonb NOT NULL,
 plan jsonb NOT NULL,
 digest text NOT NULL CHECK (digest ~ '^[a-f0-9]{64}$'),
 published_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (workspace_id,workflow_id,id),
 UNIQUE (workspace_id,workflow_id,sequence),
 UNIQUE (workspace_id,workflow_id,draft_revision),
 FOREIGN KEY (workspace_id,workflow_id) REFERENCES workflows(workspace_id,id)
);
ALTER TABLE workflows ADD CONSTRAINT active_version_owner
 FOREIGN KEY (workspace_id,id,active_version_id) REFERENCES workflow_versions(workspace_id,workflow_id,id);
CREATE FUNCTION immutable_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'published versions are immutable' USING ERRCODE='23000'; END;
$$;
CREATE TRIGGER no_version_change BEFORE UPDATE OR DELETE ON workflow_versions
 FOR EACH ROW EXECUTE FUNCTION immutable_version();
CREATE TABLE runs (
 workspace_id text NOT NULL,
 id text NOT NULL,
 workflow_id text NOT NULL,
 version_id text NOT NULL,
 state text NOT NULL DEFAULT 'accepted' CHECK (state = 'accepted'),
 input jsonb NOT NULL,
 accepted_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY (workspace_id,id),
 UNIQUE (id),
 FOREIGN KEY (workspace_id,workflow_id,version_id) REFERENCES workflow_versions(workspace_id,workflow_id,id)
);
CREATE INDEX runs_browse ON runs(workspace_id,accepted_at,id);
CREATE TABLE trigger_inbox (
 workspace_id text NOT NULL,
 workflow_id text NOT NULL,
 source text NOT NULL CHECK (source = 'manual'),
 trigger_key text NOT NULL CHECK (length(trigger_key) BETWEEN 1 AND 128),
 fingerprint text NOT NULL,
 run_id text NOT NULL,
 PRIMARY KEY (workspace_id,workflow_id,source,trigger_key),
 FOREIGN KEY (workspace_id,workflow_id) REFERENCES workflows(workspace_id,id),
 FOREIGN KEY (workspace_id,run_id) REFERENCES runs(workspace_id,id)
);
CREATE TABLE start_outbox (
 workspace_id text NOT NULL,
 run_id text PRIMARY KEY,
 attempts integer NOT NULL DEFAULT 0,
 lease_token text,
 lease_until timestamptz,
 next_try timestamptz NOT NULL DEFAULT now(),
 dispatched_at timestamptz,
 FOREIGN KEY (workspace_id,run_id) REFERENCES runs(workspace_id,id)
);
CREATE INDEX outbox_ready ON start_outbox(next_try,run_id) WHERE dispatched_at IS NULL;
CREATE TABLE runtime_receipts (
 workspace_id text NOT NULL,
 run_id text PRIMARY KEY,
 runtime_id text NOT NULL UNIQUE,
 recorded_at timestamptz NOT NULL DEFAULT now(),
 FOREIGN KEY (workspace_id,run_id) REFERENCES runs(workspace_id,id)
);
