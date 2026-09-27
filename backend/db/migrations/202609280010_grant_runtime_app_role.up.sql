-- The database administrator creates lanverse_app as a non-login role before
-- migration and grants it to a separate, non-owning application login.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_roles
    WHERE rolname = 'lanverse_app' AND NOT rolcanlogin AND NOT rolsuper
  ) THEN
    RAISE EXCEPTION 'lanverse_app must be a pre-provisioned NOLOGIN, NOSUPERUSER role';
  END IF;
END $$;

GRANT USAGE ON SCHEMA infra, identity, workspace, catalog, operation, media TO lanverse_app;
-- The ledger ACL migration already grants billing schema USAGE. Audit has its
-- own append-only ACL migration; neither grant is broadened here.

-- API commands append events; Relay locks and acknowledges them; Worker prunes
-- published events. Column UPDATE also permits PostgreSQL row locks.
GRANT SELECT, INSERT, UPDATE (published_at, update_time), DELETE
  ON TABLE infra.outbox TO lanverse_app;
GRANT SELECT, INSERT, UPDATE (update_time), DELETE
  ON TABLE infra.processed_event TO lanverse_app;

GRANT SELECT, INSERT, UPDATE ON TABLE identity."user" TO lanverse_app;
-- Organization, operation and media rows are locked with FOR SHARE by current
-- commands. PostgreSQL requires UPDATE on at least one column for that lock.
GRANT SELECT, INSERT, UPDATE (update_time) ON TABLE workspace.organization TO lanverse_app;
GRANT SELECT, INSERT, UPDATE ON TABLE workspace.project TO lanverse_app;
GRANT SELECT ON TABLE workspace.style_preset TO lanverse_app;

GRANT SELECT, INSERT, UPDATE ON TABLE catalog.provider, catalog.provider_credential,
  catalog.model_profile TO lanverse_app;
GRANT SELECT, INSERT ON TABLE catalog.capability, catalog.model_profile_version,
  catalog.price_rule_version TO lanverse_app;

GRANT SELECT, INSERT, UPDATE ON TABLE billing.budget TO lanverse_app;
GRANT SELECT ON TABLE billing.reservation TO lanverse_app;

GRANT SELECT, INSERT ON TABLE operation.batch, operation.operation_input TO lanverse_app;
GRANT SELECT, INSERT, UPDATE (update_time) ON TABLE operation.operation TO lanverse_app;

GRANT SELECT, INSERT, UPDATE (update_time) ON TABLE media.media_asset TO lanverse_app;
GRANT SELECT, INSERT ON TABLE media.rendition TO lanverse_app;
