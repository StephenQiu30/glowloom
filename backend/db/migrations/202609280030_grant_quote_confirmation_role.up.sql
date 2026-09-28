-- Quote confirmation writes a held reservation and advances its operation
-- or batch after a user confirms. Keep the runtime role scoped to those writes.
GRANT INSERT ON TABLE billing.reservation TO lanverse_app;
GRANT UPDATE (reservation_id, confirmed_at, confirmed_by)
  ON TABLE operation.operation TO lanverse_app;
GRANT UPDATE (status, total_count, quote_total_micros, update_time)
  ON TABLE operation.batch TO lanverse_app;
