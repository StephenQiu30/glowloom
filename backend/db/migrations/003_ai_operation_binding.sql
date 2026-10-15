-- Additive operation bindings. Apply after 002 in a reviewed migration window.
-- Existing operations are intentionally not inferred from mutable configuration.
BEGIN;
CREATE TABLE operation.provider_binding (
    operation_id uuid PRIMARY KEY REFERENCES operation.operation(id),
    release_id uuid NOT NULL REFERENCES catalog.model_release(id),
    create_time timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE operation.provider_credential_binding (
    operation_id uuid PRIMARY KEY REFERENCES operation.provider_binding(operation_id),
    credential_id uuid NOT NULL REFERENCES catalog.provider_credential(id),
    create_time timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT,INSERT ON operation.provider_binding,operation.provider_credential_binding TO lanverse_app;
CREATE TABLE operation.provider_result (
 operation_id uuid PRIMARY KEY REFERENCES operation.provider_credential_binding(operation_id),
 result jsonb NOT NULL CHECK(jsonb_typeof(result)='object' AND octet_length(result::text)<=2097152),
 update_time timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT,INSERT,UPDATE ON operation.provider_result TO lanverse_app;
CREATE TABLE operation.provider_trial (
 operation_id uuid PRIMARY KEY REFERENCES operation.provider_binding(operation_id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 max_charge_micros bigint NOT NULL CHECK(max_charge_micros>=0),
 supplier_max_micros bigint NOT NULL CHECK(supplier_max_micros BETWEEN 0 AND 1000000000),
 supplier_currency text NOT NULL CHECK(supplier_currency IN ('CNY','USD'))
);
GRANT SELECT,INSERT ON operation.provider_trial TO lanverse_app;
COMMIT;
