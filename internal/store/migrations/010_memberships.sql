CREATE TABLE workspace_memberships (
    workspace_id text NOT NULL REFERENCES workspaces(id),
    issuer text NOT NULL CHECK (length(issuer) BETWEEN 1 AND 2048),
    subject text NOT NULL CHECK (length(subject) BETWEEN 1 AND 255),
    role text NOT NULL CHECK (role IN ('viewer', 'editor', 'operator', 'owner')),
    publish boolean NOT NULL DEFAULT false,
    execute boolean NOT NULL DEFAULT false,
    data_read boolean NOT NULL DEFAULT false,
    PRIMARY KEY (workspace_id, issuer, subject)
);
