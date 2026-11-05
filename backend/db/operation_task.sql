-- A resumed batch acknowledges failures observed before the operator reopened admission.
ALTER TABLE operation.batch ADD COLUMN resumed_at timestamptz;
GRANT SELECT(resumed_at), UPDATE(resumed_at) ON operation.batch TO glowloom_app;
