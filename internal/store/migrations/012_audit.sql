CREATE TABLE audit_cursors (
    workspace_id text PRIMARY KEY,
    sequence bigint NOT NULL CHECK (sequence BETWEEN 1 AND 9007199254740991)
);
CREATE TABLE audit_records (
    workspace_id text NOT NULL,
    sequence bigint NOT NULL CHECK (sequence BETWEEN 1 AND 9007199254740991),
    actor text NOT NULL CHECK (length(actor) BETWEEN 1 AND 100),
    request_id text NOT NULL CHECK (length(request_id) <= 100),
    action text NOT NULL CHECK (action IN (
        'workflow.created','workflow.draft_saved','workflow.published','workflow.imported',
        'credential.created','credential.rotated','credential.revoked','credential.granted',
        'membership.provisioned','membership.set','membership.removed',
        'run.admitted','run.cancel_requested','run.resolved','schedule.saved'
    )),
    target_id text NOT NULL CHECK (length(target_id) BETWEEN 1 AND 128),
    details jsonb NOT NULL CHECK (jsonb_typeof(details)='object' AND octet_length(details::text)<=2048),
    recorded_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (workspace_id,sequence)
);
CREATE FUNCTION reject_audit_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'audit records are append-only' USING ERRCODE='42501';
END;
$$;
CREATE TRIGGER audit_records_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON audit_records
FOR EACH STATEMENT EXECUTE FUNCTION reject_audit_mutation();
REVOKE UPDATE, DELETE, TRUNCATE ON audit_records FROM PUBLIC;
