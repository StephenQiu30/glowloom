REVOKE UPDATE (cancel_requested_at)
  ON TABLE operation.batch FROM lanverse_app;

ALTER TABLE operation.batch
  DROP COLUMN cancel_requested_at;
