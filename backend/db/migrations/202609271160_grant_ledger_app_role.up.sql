-- Provision lanverse_app as a non-login role before applying migrations.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_roles
    WHERE rolname = 'lanverse_app' AND NOT rolcanlogin AND NOT rolsuper
  ) THEN
    RAISE EXCEPTION 'lanverse_app must be a pre-provisioned NOLOGIN, NOSUPERUSER role';
  END IF;
END $$;

GRANT USAGE ON SCHEMA billing TO lanverse_app;
REVOKE ALL PRIVILEGES ON TABLE billing.ledger_entry FROM lanverse_app;
GRANT INSERT, SELECT ON TABLE billing.ledger_entry TO lanverse_app;
GRANT UPDATE (is_delete, update_time) ON TABLE billing.ledger_entry TO lanverse_app;
