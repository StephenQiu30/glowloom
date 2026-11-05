CREATE TABLE IF NOT EXISTS media.storage_idempotency_lock (
 namespace text NOT NULL,
 actor_id uuid NOT NULL,
 request_key uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CONSTRAINT storage_idempotency_lock_namespace_check CHECK (namespace IN ('purge','package','transfer','upload','personal_upload','library')),
 CONSTRAINT storage_idempotency_lock_pkey PRIMARY KEY (namespace, actor_id, request_key)
);

GRANT SELECT, INSERT, UPDATE ON TABLE media.storage_idempotency_lock TO glowloom_app;

INSERT INTO media.storage_schema_version(version) VALUES (2) ON CONFLICT (version) DO NOTHING;
