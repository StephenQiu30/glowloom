CREATE INDEX ix_operation_due_quote
  ON operation.operation (quote_expires_at, id)
  WHERE status = 'quoted' AND NOT is_delete;

CREATE INDEX ix_operation_live_batch_status
  ON operation.operation (batch_id, status)
  WHERE batch_id IS NOT NULL AND NOT is_delete;

CREATE INDEX ix_batch_quoted
  ON operation.batch (create_time, id)
  WHERE status = 'quoted' AND NOT is_delete;
