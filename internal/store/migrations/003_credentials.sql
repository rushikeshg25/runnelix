CREATE TABLE credentials (
 workspace_id text NOT NULL REFERENCES workspaces(id),
 id text NOT NULL,
 name text NOT NULL CHECK(length(name) BETWEEN 1 AND 120),
 kind text NOT NULL CHECK(kind IN ('bearer','webhook-hmac')),
 destination text NOT NULL,
 ciphertext bytea NOT NULL,
 key_id text NOT NULL,
 revoked boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(workspace_id,id)
);
CREATE TABLE credential_grants (
 workspace_id text NOT NULL,
 credential_id text NOT NULL,
 workflow_id text NOT NULL,
 PRIMARY KEY(workspace_id,credential_id,workflow_id),
 FOREIGN KEY(workspace_id,credential_id) REFERENCES credentials(workspace_id,id),
 FOREIGN KEY(workspace_id,workflow_id) REFERENCES workflows(workspace_id,id)
);
