REVOKE UPDATE (status, total_count, quote_total_micros, update_time)
  ON TABLE operation.batch FROM lanverse_app;
REVOKE UPDATE (reservation_id, confirmed_at, confirmed_by)
  ON TABLE operation.operation FROM lanverse_app;
REVOKE INSERT ON TABLE billing.reservation FROM lanverse_app;
