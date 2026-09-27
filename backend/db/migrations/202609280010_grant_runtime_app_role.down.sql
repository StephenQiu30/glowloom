REVOKE SELECT, INSERT ON TABLE media.rendition FROM lanverse_app;
REVOKE SELECT, INSERT, UPDATE (update_time) ON TABLE media.media_asset FROM lanverse_app;

REVOKE SELECT, INSERT, UPDATE (update_time) ON TABLE operation.operation FROM lanverse_app;
REVOKE SELECT, INSERT ON TABLE operation.batch, operation.operation_input FROM lanverse_app;

REVOKE SELECT ON TABLE billing.reservation FROM lanverse_app;
REVOKE SELECT, INSERT, UPDATE ON TABLE billing.budget FROM lanverse_app;

REVOKE SELECT, INSERT ON TABLE catalog.capability, catalog.model_profile_version,
  catalog.price_rule_version FROM lanverse_app;
REVOKE SELECT, INSERT, UPDATE ON TABLE catalog.provider, catalog.provider_credential,
  catalog.model_profile FROM lanverse_app;

REVOKE SELECT ON TABLE workspace.style_preset FROM lanverse_app;
REVOKE SELECT, INSERT, UPDATE ON TABLE workspace.project FROM lanverse_app;
REVOKE SELECT, INSERT, UPDATE (update_time) ON TABLE workspace.organization FROM lanverse_app;
REVOKE SELECT, INSERT, UPDATE ON TABLE identity."user" FROM lanverse_app;

REVOKE SELECT, INSERT, UPDATE (update_time), DELETE ON TABLE infra.processed_event FROM lanverse_app;
REVOKE SELECT, INSERT, UPDATE (published_at, update_time), DELETE ON TABLE infra.outbox FROM lanverse_app;

REVOKE USAGE ON SCHEMA infra, identity, workspace, catalog, operation, media FROM lanverse_app;
