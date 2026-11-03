-- Agent runtime storage. Run explicitly as the schema owner; never on application startup.
-- Safe to apply again. PostgreSQL remains the only durable data source.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_roles WHERE rolname = 'glowloom_agent_runtime'
        AND NOT rolcanlogin AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolbypassrls
    ) THEN
        RAISE EXCEPTION 'glowloom_agent_runtime must be pre-provisioned NOLOGIN with no administrative privileges';
    END IF;
END
$$;
CREATE SCHEMA IF NOT EXISTS agent;
CREATE TABLE IF NOT EXISTS agent.runtime_session (
    id uuid PRIMARY KEY,
    lock_key integer GENERATED ALWAYS AS IDENTITY UNIQUE,
    storage_abi integer NOT NULL DEFAULT 1 CHECK (storage_abi > 0),
    sdk_version text NOT NULL DEFAULT '1.3.0',
    owner_epoch bigint NOT NULL DEFAULT 0 CHECK (owner_epoch >= 0 AND owner_epoch <= 9007199254740991),
    last_seq bigint NOT NULL DEFAULT 0 CHECK (last_seq >= 0 AND last_seq <= 9007199254740991),
    last_hash text NOT NULL DEFAULT '' CHECK (last_hash = '' OR last_hash ~ '^[a-f0-9]{64}$'),
    next_id bigint NOT NULL DEFAULT 1 CHECK (next_id >= 1 AND next_id <= 9007199254740991),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS agent.runtime_commit (
    session_id uuid NOT NULL REFERENCES agent.runtime_session(id) ON DELETE CASCADE,
    seq bigint NOT NULL CHECK (seq > 0 AND seq <= 9007199254740991),
    owner_epoch bigint NOT NULL CHECK (owner_epoch > 0),
    writes bytea NOT NULL,
    previous_hash text NOT NULL CHECK (previous_hash = '' OR previous_hash ~ '^[a-f0-9]{64}$'),
    content_hash text NOT NULL CHECK (content_hash ~ '^[a-f0-9]{64}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id, seq)
);
GRANT USAGE ON SCHEMA agent TO glowloom_agent_runtime;
GRANT SELECT, INSERT, UPDATE ON agent.runtime_session TO glowloom_agent_runtime;
GRANT SELECT, INSERT ON agent.runtime_commit TO glowloom_agent_runtime;
GRANT USAGE ON SEQUENCE agent.runtime_session_lock_key_seq TO glowloom_agent_runtime;

-- Go control data is separate from SDK checkpoints and writable only by Go.
ALTER TABLE agent.session ADD COLUMN IF NOT EXISTS engine text NOT NULL DEFAULT 'image-agent-json-v1'
    CHECK (engine IN ('image-agent-json-v1','pi-durable-v1'));
CREATE TABLE IF NOT EXISTS agent.harness_run (
    id uuid PRIMARY KEY,
    session_id uuid NOT NULL REFERENCES agent.session(id),
    project_id uuid NOT NULL REFERENCES workspace.project(id),
    actor_id uuid NOT NULL REFERENCES identity."user"(id),
    org_id uuid NOT NULL REFERENCES workspace.organization(id),
    scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
    runtime_session_id uuid NOT NULL UNIQUE,
    request_id uuid NOT NULL,
    input_hash text NOT NULL,
    revision bigint NOT NULL CHECK(revision>0),
    control_revision bigint NOT NULL CHECK(control_revision>0),
    status text NOT NULL,
    state jsonb NOT NULL CHECK(jsonb_typeof(state)='object'),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(session_id,request_id)
);
CREATE TABLE IF NOT EXISTS agent.harness_candidate (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES agent.harness_run(id),
    request_id uuid NOT NULL,
    purpose text NOT NULL CHECK(purpose IN ('planning','execution')),
    content_hash text NOT NULL,
    manifest jsonb NOT NULL,
    plan_revision bigint NOT NULL,
    UNIQUE(run_id,request_id)
);
CREATE TABLE IF NOT EXISTS agent.harness_command (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES agent.harness_run(id),
    request_id uuid NOT NULL,
    input_hash text NOT NULL,
    action text NOT NULL,
    receipt jsonb NOT NULL,
    delivery jsonb NOT NULL,
    delivery_state text NOT NULL CHECK(delivery_state IN ('pending','leased','acknowledged','superseded')),
    claim_token uuid,
    claim_until timestamptz,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    attempts integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(run_id,request_id)
);
CREATE INDEX IF NOT EXISTS harness_command_pending ON agent.harness_command(next_attempt_at,created_at,id) WHERE delivery_state IN ('pending','leased');
CREATE TABLE IF NOT EXISTS agent.harness_approval (
    id uuid PRIMARY KEY REFERENCES agent.harness_command(id),
    run_id uuid NOT NULL REFERENCES agent.harness_run(id),
    candidate_id uuid NOT NULL REFERENCES agent.harness_candidate(id),
    actor_id uuid NOT NULL REFERENCES identity."user"(id),
    max_micros bigint NOT NULL CHECK(max_micros>0),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS agent.harness_admission (
    id uuid PRIMARY KEY,
    run_id uuid NOT NULL REFERENCES agent.harness_run(id),
    request_id uuid NOT NULL,
    input_hash text NOT NULL,
    action text NOT NULL,
    candidate_id uuid NOT NULL REFERENCES agent.harness_candidate(id),
    control_revision bigint NOT NULL CHECK(control_revision>0),
    max_micros bigint NOT NULL CHECK(max_micros>=0),
    status text NOT NULL DEFAULT 'in_flight' CHECK(status IN ('in_flight','settled','unknown')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(run_id,request_id)
);
GRANT SELECT,INSERT,UPDATE ON agent.harness_run,agent.harness_command TO glowloom_app;
GRANT SELECT,INSERT ON agent.harness_candidate,agent.harness_approval,agent.harness_admission TO glowloom_app;
