-- Install in an empty or explicitly migrated business database.
CREATE TABLE infra.task_work (
    kind text NOT NULL,
    business_id text NOT NULL,
    next_run_at timestamptz NOT NULL,
    owner text,
    epoch bigint NOT NULL DEFAULT 0,
    lease_until timestamptz,
    attempt integer NOT NULL DEFAULT 0,
    reference jsonb NOT NULL DEFAULT 'null',
    failures integer NOT NULL DEFAULT 0,
    failed_at timestamptz,
    stop_requested_at timestamptz,
    completed_at timestamptz,
    PRIMARY KEY (kind, business_id),
    CHECK (kind <> '' AND business_id <> ''),
    CHECK ((owner IS NULL) = (lease_until IS NULL))
);
CREATE INDEX task_work_due ON infra.task_work (kind, next_run_at) WHERE completed_at IS NULL AND failed_at IS NULL;

-- Exactly one cursor for each periodic business sweep. Restarting a process
-- never resets its due time. The worker checks existing expiries on first claim.
INSERT INTO infra.task_work (kind,business_id,next_run_at) VALUES
 ('maintenance.partitions','periodic',now()),
 ('maintenance.outbox','periodic',now()),
 ('maintenance.inbox','periodic',now()),
 -- Content deletion remains disabled until an operator explicitly enables it.
 ('workspace.retention','periodic','infinity'),
 ('operation.quote_expiry','periodic',now());

-- Administrator trials previously kept their owner-scoped state in external
-- execution history. This is the business record for new local trial requests.
CREATE TABLE operation.development_trial (
 id text PRIMARY KEY,
 input jsonb NOT NULL,
 state jsonb NOT NULL,
 phase text NOT NULL CHECK(phase IN ('ready','dispatching','poll','done')),
 generation bigint NOT NULL DEFAULT 1,
 polls integer NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE ON infra.task_work, operation.development_trial TO glowloom_app;
