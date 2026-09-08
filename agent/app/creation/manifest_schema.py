"""Initial execution plans are durable facts, never a live registry projection."""

import hashlib

MANIFEST_SCHEMA = """
CREATE TABLE creation_manifests (
    command_id uuid PRIMARY KEY REFERENCES creation_executions(command_id),
    manifest jsonb NOT NULL CHECK (jsonb_typeof(manifest) = 'object'),
    manifest_hash text NOT NULL CHECK (manifest_hash ~ '^[0-9a-f]{64}$'),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE FUNCTION creation_guard_manifest() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'creation manifest is immutable';
END;
$$;
CREATE TRIGGER creation_manifest_immutable BEFORE UPDATE OR DELETE ON creation_manifests
    FOR EACH ROW EXECUTE FUNCTION creation_guard_manifest();
"""
MANIFEST_SCHEMA_HASH = hashlib.sha256(MANIFEST_SCHEMA.encode()).hexdigest()
