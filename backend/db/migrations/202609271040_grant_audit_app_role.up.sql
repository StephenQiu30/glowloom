-- The database administrator provisions this non-login role before migration.
-- Application logins inherit it but must not own the audit table.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_roles
    WHERE rolname = 'lanverse_app' AND NOT rolcanlogin AND NOT rolsuper
  ) THEN
    RAISE EXCEPTION 'lanverse_app must be a pre-provisioned NOLOGIN, NOSUPERUSER role';
  END IF;
END $$;

REVOKE ALL PRIVILEGES ON SCHEMA audit FROM lanverse_app;
GRANT USAGE ON SCHEMA audit TO lanverse_app;

REVOKE ALL PRIVILEGES ON TABLE audit.audit_log FROM lanverse_app;
GRANT INSERT, SELECT ON TABLE audit.audit_log TO lanverse_app;
