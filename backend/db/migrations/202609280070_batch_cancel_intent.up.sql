ALTER TABLE operation.batch
  ADD COLUMN cancel_requested_at timestamptz;

GRANT UPDATE (cancel_requested_at)
  ON TABLE operation.batch TO lanverse_app;
