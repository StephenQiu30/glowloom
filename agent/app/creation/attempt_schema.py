"""Append-only migration for observed inference attempts; no fabricated legacy history."""

import hashlib

ATTEMPT_SCHEMA = """
CREATE TABLE creation_attempts (
    id uuid PRIMARY KEY,
    step_id uuid NOT NULL REFERENCES creation_steps(id),
    attempt_no integer NOT NULL CHECK (attempt_no > 0),
    fence bigint NOT NULL CHECK (fence > 0),
    input_hash text NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
    state text NOT NULL CHECK (state IN ('running', 'succeeded', 'unknown')),
    started_at timestamptz NOT NULL,
    execution_deadline timestamptz NOT NULL,
    lease_expires_at timestamptz NOT NULL,
    finished_at timestamptz,
    result_hash text CHECK (result_hash ~ '^[0-9a-f]{64}$'),
    last_error text CHECK (last_error IN (
        'harness_response_unknown', 'harness_result_invalid', 'attempt_cancelled', 'attempt_expired'
    )),
    usage_status text NOT NULL DEFAULT 'unknown' CHECK (usage_status = 'unknown'),
    UNIQUE (step_id, attempt_no),
    UNIQUE (step_id, fence),
    UNIQUE (step_id, id),
    CHECK (started_at < execution_deadline AND execution_deadline < lease_expires_at),
    CHECK ((state = 'running') = (finished_at IS NULL)),
    CHECK ((state = 'succeeded') = (result_hash IS NOT NULL)),
    CHECK ((state = 'unknown') = (last_error IS NOT NULL)),
    CHECK (finished_at IS NULL OR finished_at >= started_at)
);
ALTER TABLE creation_steps ADD COLUMN current_attempt_id uuid;
ALTER TABLE creation_steps ADD CONSTRAINT creation_step_current_attempt
    FOREIGN KEY (id, current_attempt_id) REFERENCES creation_attempts(step_id, id);
ALTER TABLE creation_drafts ADD COLUMN attempt_id uuid;
ALTER TABLE creation_drafts ADD CONSTRAINT creation_draft_attempt
    FOREIGN KEY (step_id, attempt_id) REFERENCES creation_attempts(step_id, id);
ALTER TABLE creation_drafts ADD CONSTRAINT creation_draft_attempt_identity
    UNIQUE (step_id, id, attempt_id);
ALTER TABLE creation_result_outbox ADD COLUMN attempt_id uuid;
ALTER TABLE creation_result_outbox ADD CONSTRAINT creation_result_attempt
    FOREIGN KEY (step_id, attempt_id) REFERENCES creation_attempts(step_id, id);
ALTER TABLE creation_result_outbox ADD CONSTRAINT creation_result_draft_attempt
    FOREIGN KEY (step_id, draft_id, attempt_id)
    REFERENCES creation_drafts(step_id, id, attempt_id);

CREATE FUNCTION creation_guard_attempt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF (NEW.id, NEW.step_id, NEW.attempt_no, NEW.fence, NEW.input_hash, NEW.started_at,
        NEW.execution_deadline, NEW.lease_expires_at, NEW.usage_status)
        IS DISTINCT FROM
       (OLD.id, OLD.step_id, OLD.attempt_no, OLD.fence, OLD.input_hash, OLD.started_at,
        OLD.execution_deadline, OLD.lease_expires_at, OLD.usage_status)
       OR (OLD.state <> 'running' AND NEW IS DISTINCT FROM OLD) THEN
        RAISE EXCEPTION 'creation attempt identity or terminal record is immutable'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER creation_attempt_immutable BEFORE UPDATE ON creation_attempts
    FOR EACH ROW EXECUTE FUNCTION creation_guard_attempt();
"""
ATTEMPT_SCHEMA_HASH = hashlib.sha256(ATTEMPT_SCHEMA.encode()).hexdigest()
