"""Append-only migrations for the trusted execution store."""

import hashlib

EXECUTION_SCHEMA = """
CREATE TABLE creation_executions (
    command_id uuid PRIMARY KEY REFERENCES creation_commands(command_id),
    release_hash text NOT NULL CHECK (release_hash ~ '^[0-9a-f]{64}$'),
    call_limit integer NOT NULL CHECK (call_limit BETWEEN 1 AND 1000),
    reserved_calls integer NOT NULL DEFAULT 0 CHECK (reserved_calls BETWEEN 0 AND call_limit),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE creation_steps (
    id uuid PRIMARY KEY,
    command_id uuid NOT NULL REFERENCES creation_executions(command_id),
    step_key text NOT NULL,
    input_hash text NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
    task jsonb NOT NULL,
    state text NOT NULL CHECK (state IN ('running','needs_review','unknown')),
    fence bigint NOT NULL CHECK (fence > 0),
    lease_until timestamptz,
    usage_status text NOT NULL DEFAULT 'unknown' CHECK (usage_status = 'unknown'),
    last_error text,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    updated_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(command_id, step_key)
);
CREATE TABLE creation_drafts (
    id uuid PRIMARY KEY,
    step_id uuid NOT NULL UNIQUE REFERENCES creation_steps(id),
    candidate_hash text NOT NULL CHECK (candidate_hash ~ '^[0-9a-f]{64}$'),
    result_hash text NOT NULL CHECK (result_hash ~ '^[0-9a-f]{64}$'),
    result jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE creation_output_bindings (
    step_id uuid NOT NULL REFERENCES creation_steps(id),
    output_role text NOT NULL CHECK (output_role = 'candidate'),
    item_key text NOT NULL CHECK (item_key = 'primary'),
    draft_id uuid NOT NULL UNIQUE REFERENCES creation_drafts(id),
    PRIMARY KEY(step_id, output_role, item_key)
);
CREATE TABLE creation_result_outbox (
    event_id uuid PRIMARY KEY,
    step_id uuid NOT NULL UNIQUE REFERENCES creation_steps(id),
    draft_id uuid NOT NULL REFERENCES creation_drafts(id),
    event_type text NOT NULL CHECK (event_type = 'result_ready'),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
"""
EXECUTION_SCHEMA_HASH = hashlib.sha256(EXECUTION_SCHEMA.encode()).hexdigest()
