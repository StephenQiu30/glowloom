-- Offline migration. A deployment administrator provisions the restricted login
-- glowloom_agent beforehand; the service never creates roles, schemas or tables.
BEGIN;
CREATE SCHEMA IF NOT EXISTS agent;
CREATE TABLE agent.runtime_sessions (
    session_id text PRIMARY KEY,
    owner_id text NOT NULL,
    owner_epoch bigint NOT NULL CHECK (owner_epoch > 0),
    lease_until timestamptz NOT NULL,
    last_id bigint NOT NULL DEFAULT 1 CHECK (last_id BETWEEN 1 AND 9007199254740991),
    commit_seq bigint NOT NULL DEFAULT 0 CHECK (commit_seq BETWEEN 0 AND 9007199254740991)
);
-- JSON encoded text preserves JS strings including lone surrogates and NUL.
-- Indexed identity columns store JSON encoded scalar strings for the same reason.
CREATE TABLE agent.runtime_records (
    session_id text NOT NULL REFERENCES agent.runtime_sessions,
    id bigint NOT NULL CHECK (id BETWEEN 1 AND 9007199254740991),
    record_type text NOT NULL CHECK (record_type IN ('conversation','entry','task','submission','document')),
    body text NOT NULL,
    commit_seq bigint NOT NULL,
    conversation_id bigint,
    owner_conversation_id bigint,
    owner_task_id bigint,
    parent_id bigint,
    parent_at bigint,
    head_id bigint,
    kind_key text,
    status text,
    background boolean,
    abort_requested boolean,
    request_key text,
    scope_key text,
    address_key text,
    created_at_seq bigint,
    retired_at_seq bigint,
    PRIMARY KEY (session_id, id)
);
CREATE INDEX runtime_records_scan ON agent.runtime_records(session_id, record_type, id);
CREATE INDEX runtime_records_conversation ON agent.runtime_records(session_id, record_type, conversation_id, id);
CREATE INDEX runtime_records_owner_task ON agent.runtime_records(session_id, owner_task_id, id) WHERE owner_task_id IS NOT NULL;
-- Hash exclusion rechecks exact equality and avoids btree's key-size limit for
-- arbitrary SDK strings. No digest is treated as a durable identity.
ALTER TABLE agent.runtime_records ADD CONSTRAINT runtime_submission_request
  EXCLUDE USING hash ((ARRAY[session_id, conversation_id::text, request_key]) WITH =) WHERE (request_key IS NOT NULL);
CREATE INDEX runtime_request_lookup ON agent.runtime_records USING hash(request_key) WHERE request_key IS NOT NULL;
ALTER TABLE agent.runtime_records ADD CONSTRAINT runtime_live_document
  EXCLUDE USING hash ((ARRAY[session_id,address_key]) WITH =) WHERE (address_key IS NOT NULL AND retired_at_seq IS NULL);
CREATE INDEX runtime_document_address ON agent.runtime_records USING hash(address_key) WHERE address_key IS NOT NULL;
CREATE INDEX runtime_document_scope ON agent.runtime_records(session_id,scope_key,id) WHERE scope_key IS NOT NULL;
CREATE TABLE agent.runtime_document_versions (
    session_id text NOT NULL,
    document_id bigint NOT NULL,
    commit_seq bigint NOT NULL,
    version integer NOT NULL,
    content text NOT NULL,
    deltas_since_base integer NOT NULL,
    PRIMARY KEY(session_id,document_id,commit_seq),
    FOREIGN KEY(session_id,document_id) REFERENCES agent.runtime_records
);
CREATE TABLE agent.runtime_events (
    session_id text NOT NULL REFERENCES agent.runtime_sessions,
    commit_seq bigint NOT NULL,
    event_ordinal integer NOT NULL CHECK (event_ordinal > 0),
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    PRIMARY KEY (session_id,commit_seq,event_ordinal)
);
REVOKE ALL ON agent.runtime_sessions, agent.runtime_records, agent.runtime_document_versions, agent.runtime_events FROM PUBLIC;
GRANT USAGE ON SCHEMA agent TO glowloom_agent;
GRANT SELECT,INSERT,UPDATE ON agent.runtime_sessions, agent.runtime_records TO glowloom_agent;
GRANT SELECT,INSERT,DELETE ON agent.runtime_document_versions, agent.runtime_events TO glowloom_agent;
COMMIT;
