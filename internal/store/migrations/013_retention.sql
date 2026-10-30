ALTER TABLE runs ADD COLUMN retention_hold boolean NOT NULL DEFAULT false;
ALTER TABLE runs ADD COLUMN payloads_expired_at timestamptz;
ALTER TABLE runs ADD COLUMN history_expired_at timestamptz;
ALTER TABLE payloads ALTER COLUMN data DROP NOT NULL;
ALTER TABLE payloads ADD COLUMN expired_at timestamptz;
ALTER TABLE payloads ADD CONSTRAINT payload_expiry CHECK ((data IS NULL) = (expired_at IS NOT NULL));
CREATE INDEX runs_retention ON runs(workspace_id,updated_at,id)
 WHERE state IN ('succeeded','failed','cancelled') AND NOT retention_hold AND history_expired_at IS NULL;
ALTER TABLE audit_records DROP CONSTRAINT audit_records_action_check;
ALTER TABLE audit_records ADD CONSTRAINT audit_records_action_check CHECK (action IN (
 'workflow.created','workflow.draft_saved','workflow.published','workflow.imported',
 'credential.created','credential.rotated','credential.revoked','credential.granted',
 'membership.provisioned','membership.set','membership.removed',
 'run.admitted','run.cancel_requested','run.resolved','schedule.saved',
 'run.payloads_expired','run.history_expired','run.retention_hold_set'
));
-- Keep updates immutable. Permit history deletion only after the locked parent
-- has been marked expired by retention. Audit records keep their own guard.
CREATE FUNCTION retained_history_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'DELETE' AND EXISTS (
   SELECT 1 FROM runs WHERE workspace_id=OLD.workspace_id AND id=OLD.run_id
   AND history_expired_at IS NOT NULL AND NOT retention_hold
   AND state IN ('succeeded','failed','cancelled')
 ) THEN RETURN OLD; END IF;
 RAISE EXCEPTION 'run history is immutable until retention expiry' USING ERRCODE='42501';
END;
$$;
DROP TRIGGER no_attempt_change ON attempts;
CREATE TRIGGER no_attempt_change BEFORE UPDATE OR DELETE ON attempts FOR EACH ROW EXECUTE FUNCTION retained_history_guard();
DROP TRIGGER no_event_change ON run_events;
CREATE TRIGGER no_event_change BEFORE UPDATE OR DELETE ON run_events FOR EACH ROW EXECUTE FUNCTION retained_history_guard();
DROP TRIGGER no_resolution_change ON operator_resolutions;
CREATE TRIGGER no_resolution_change BEFORE UPDATE OR DELETE ON operator_resolutions FOR EACH ROW EXECUTE FUNCTION retained_history_guard();
-- Serialize late writes against retention's parent FOR UPDATE lock. Expired
-- data cannot be repopulated by a stale activity or delayed delivery.
CREATE FUNCTION retained_run_insert_guard() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE expired boolean;
BEGIN
 SELECT payloads_expired_at IS NOT NULL INTO expired FROM runs
 WHERE workspace_id=NEW.workspace_id AND id=NEW.run_id FOR KEY SHARE;
 IF expired THEN RAISE EXCEPTION 'run data expired' USING ERRCODE='42501'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER retained_payload BEFORE INSERT ON payloads FOR EACH ROW EXECUTE FUNCTION retained_run_insert_guard();
CREATE TRIGGER retained_invocation BEFORE INSERT ON node_invocations FOR EACH ROW EXECUTE FUNCTION retained_run_insert_guard();
CREATE TRIGGER retained_attempt BEFORE INSERT ON attempts FOR EACH ROW EXECUTE FUNCTION retained_run_insert_guard();
CREATE TRIGGER retained_event BEFORE INSERT ON run_events FOR EACH ROW EXECUTE FUNCTION retained_run_insert_guard();
CREATE TRIGGER retained_uncertain BEFORE INSERT ON uncertain_invocations FOR EACH ROW EXECUTE FUNCTION retained_run_insert_guard();
CREATE TRIGGER retained_resolution BEFORE INSERT ON operator_resolutions FOR EACH ROW EXECUTE FUNCTION retained_run_insert_guard();
