ALTER TABLE runs ADD COLUMN projection_checked_at timestamptz NOT NULL DEFAULT 'epoch';
CREATE INDEX runs_projection_recovery_idx ON runs(projection_checked_at,id) WHERE execution_mode='execute' AND state NOT IN ('succeeded','failed','cancelled');
