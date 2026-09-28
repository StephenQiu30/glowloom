CREATE INDEX ix_batch_starter_confirmed
  ON operation.batch (update_time, id)
  WHERE status = 'confirmed' AND NOT is_delete;

CREATE INDEX ix_operation_batch_active
  ON operation.operation (batch_id, status)
  WHERE batch_id IS NOT NULL AND NOT is_delete;
