"""Append-only failure receipts and one-attempt operator recovery authorizations."""

import hashlib

RECOVERY_SCHEMA = """
ALTER TABLE creation_steps DROP CONSTRAINT creation_steps_state_check;
ALTER TABLE creation_steps ADD CONSTRAINT creation_steps_state_check
    CHECK (state IN ('running','needs_review','unknown','failed'));
ALTER TABLE creation_attempts DROP CONSTRAINT creation_attempts_state_check;
ALTER TABLE creation_attempts DROP CONSTRAINT creation_attempts_last_error_check;
ALTER TABLE creation_attempts DROP CONSTRAINT creation_attempts_check3;
ALTER TABLE creation_attempts ADD CONSTRAINT creation_attempts_state_check
    CHECK (state IN ('running','succeeded','unknown','failed'));
ALTER TABLE creation_attempts ADD CONSTRAINT creation_attempts_error_state_check
    CHECK ((state IN ('unknown','failed')) = (last_error IS NOT NULL));
ALTER TABLE creation_attempts ADD CONSTRAINT creation_attempts_error_code_check CHECK (
    last_error IS NULL OR
    (state = 'unknown' AND last_error IN
    ('harness_response_unknown','harness_result_invalid','attempt_cancelled','attempt_expired')) OR
    (state = 'failed' AND last_error IN
    ('context_insufficient','skill_release_unavailable','input_contract_invalid','candidate_contract_invalid','execution_output_budget_exceeded','execution_deadline_exceeded','structured_output_invalid'))
);
CREATE TABLE creation_attempt_failures (
    attempt_id uuid PRIMARY KEY REFERENCES creation_attempts(id),
    receipt jsonb NOT NULL,
    receipt_hash text NOT NULL CHECK (receipt_hash ~ '^[a-f0-9]{64}$'),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE creation_recoveries (
    id uuid PRIMARY KEY,
    command_id uuid NOT NULL REFERENCES creation_commands(command_id),
    step_id uuid NOT NULL,
    previous_attempt_id uuid NOT NULL UNIQUE,
    input_hash text NOT NULL CHECK (input_hash ~ '^[a-f0-9]{64}$'),
    temporal_run_id uuid NOT NULL,
    reset_event_id bigint NOT NULL CHECK (reset_event_id > 0),
    reason text NOT NULL CHECK (length(reason) BETWEEN 10 AND 500),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    consumed_at timestamptz,
    reset_run_id uuid,
    FOREIGN KEY (step_id, previous_attempt_id) REFERENCES creation_attempts(step_id, id)
);
CREATE FUNCTION creation_guard_failure_receipt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'failure receipt is immutable' USING ERRCODE = '23514';
END;
$$;
CREATE TRIGGER creation_failure_immutable BEFORE UPDATE OR DELETE ON creation_attempt_failures
    FOR EACH ROW EXECUTE FUNCTION creation_guard_failure_receipt();
CREATE FUNCTION creation_guard_recovery() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'recovery authorization is immutable' USING ERRCODE = '23514';
    END IF;
    IF (NEW.id, NEW.command_id, NEW.step_id, NEW.previous_attempt_id, NEW.input_hash,
        NEW.temporal_run_id, NEW.reset_event_id, NEW.reason, NEW.created_at)
        IS DISTINCT FROM
       (OLD.id, OLD.command_id, OLD.step_id, OLD.previous_attempt_id, OLD.input_hash,
        OLD.temporal_run_id, OLD.reset_event_id, OLD.reason, OLD.created_at)
       OR (OLD.consumed_at IS NOT NULL AND NEW.consumed_at IS DISTINCT FROM OLD.consumed_at)
       OR (OLD.reset_run_id IS NOT NULL AND NEW.reset_run_id IS DISTINCT FROM OLD.reset_run_id) THEN
        RAISE EXCEPTION 'recovery authorization is immutable' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER creation_recovery_immutable BEFORE UPDATE OR DELETE ON creation_recoveries
    FOR EACH ROW EXECUTE FUNCTION creation_guard_recovery();
"""
RECOVERY_SCHEMA_HASH = hashlib.sha256(RECOVERY_SCHEMA.encode()).hexdigest()
