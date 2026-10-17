ALTER TABLE trigger_inbox DROP CONSTRAINT trigger_inbox_source_check;
ALTER TABLE trigger_inbox ADD CONSTRAINT trigger_inbox_source_check CHECK(source IN ('manual','webhook'));
CREATE INDEX runs_pending_cancel_idx ON runs(updated_at,id) WHERE execution_mode='execute' AND cancel_requested AND state NOT IN ('succeeded','failed','cancelled');
