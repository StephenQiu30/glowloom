-- Compatible additive session protocol expansion; execute once in one transaction.
BEGIN;
SET LOCAL lock_timeout = '2s';
SET LOCAL statement_timeout = '10s';
ALTER TABLE identity.user_session
    ADD COLUMN protocol_version smallint NOT NULL DEFAULT 1 CHECK (protocol_version IN (1,2)),
    ADD COLUMN idle_timeout_seconds integer NOT NULL DEFAULT 43200 CHECK (idle_timeout_seconds > 0),
    ADD COLUMN refresh_generation bigint NOT NULL DEFAULT 0 CHECK (refresh_generation >= 0);

CREATE TABLE identity.session_refresh_token (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES identity.user_session(id) ON DELETE RESTRICT,
    generation bigint NOT NULL CHECK (generation >= 0),
    token_digest bytea NOT NULL UNIQUE CHECK (octet_length(token_digest) = 32),
    issued_at timestamptz NOT NULL,
    consumed_at timestamptz,
    revoked_at timestamptz,
    rotation_request_id uuid,
    replay_until timestamptz,
    receipt_ciphertext bytea,
    UNIQUE (session_id,generation),
    CHECK ((consumed_at IS NULL) = (rotation_request_id IS NULL)),
    CHECK ((replay_until IS NULL) = (receipt_ciphertext IS NULL)),
    CHECK (replay_until IS NULL OR (consumed_at IS NOT NULL AND replay_until > consumed_at AND replay_until <= consumed_at + interval '10 seconds' AND octet_length(receipt_ciphertext) > 0)),
    CHECK (consumed_at IS NULL OR consumed_at >= issued_at),
    CHECK (revoked_at IS NULL OR revoked_at >= issued_at)
);
CREATE UNIQUE INDEX session_refresh_one_active_idx ON identity.session_refresh_token(session_id) WHERE consumed_at IS NULL AND revoked_at IS NULL;
CREATE INDEX session_refresh_replay_cleanup_idx ON identity.session_refresh_token(replay_until,id) WHERE receipt_ciphertext IS NOT NULL;
GRANT SELECT,INSERT,DELETE ON identity.session_refresh_token TO lanverse_app;
GRANT UPDATE (consumed_at,revoked_at,rotation_request_id,replay_until,receipt_ciphertext) ON identity.session_refresh_token TO lanverse_app;
COMMIT;
