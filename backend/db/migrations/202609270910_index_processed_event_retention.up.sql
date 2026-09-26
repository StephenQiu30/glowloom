CREATE INDEX CONCURRENTLY ix_processed_event_create_time
  ON infra.processed_event (create_time, id);
