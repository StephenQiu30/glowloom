CREATE SCHEMA audit;

CREATE TABLE audit.audit_log (
  id          uuid NOT NULL,
  org_id      uuid NOT NULL,
  project_id  uuid,
  actor_id    uuid,
  actor_kind  text NOT NULL CHECK (actor_kind IN ('user', 'agent', 'system')),
  action      text NOT NULL,
  object_type text NOT NULL,
  object_id   text NOT NULL,
  before      jsonb,
  after       jsonb,
  request_id  text,
  trace_id    text,
  ip          inet,
  create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(),
  is_delete   boolean NOT NULL DEFAULT false CHECK (NOT is_delete),
  PRIMARY KEY (id, create_time)
) PARTITION BY RANGE (create_time);

CREATE INDEX ix_audit_project_time ON audit.audit_log (project_id, create_time DESC);
CREATE INDEX ix_audit_action ON audit.audit_log (action, create_time DESC);

-- Precreate the current UTC month and three future months. The default
-- partition keeps late or out-of-window events writable until maintenance.
DO $$
DECLARE
  first_month timestamp := date_trunc('month', now() AT TIME ZONE 'UTC');
  month_index integer;
  month_start timestamptz;
  month_end timestamptz;
BEGIN
  FOR month_index IN 0..3 LOOP
    month_start := (first_month + make_interval(months => month_index)) AT TIME ZONE 'UTC';
    month_end := (first_month + make_interval(months => month_index + 1)) AT TIME ZONE 'UTC';
    EXECUTE format(
      'CREATE TABLE audit.%I PARTITION OF audit.audit_log FOR VALUES FROM (%L) TO (%L)',
      'audit_log_' || to_char(month_start AT TIME ZONE 'UTC', 'YYYYMM'),
      month_start,
      month_end
    );
  END LOOP;
END $$;

CREATE TABLE audit.audit_log_default PARTITION OF audit.audit_log DEFAULT;

-- The audit writer only appends records. A separate, non-owning application
-- database role must also receive only INSERT/SELECT privileges at deployment.
CREATE FUNCTION audit.reject_audit_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'audit.audit_log is append-only' USING ERRCODE = '42501';
END $$;

CREATE TRIGGER audit_log_append_only
BEFORE UPDATE OR DELETE ON audit.audit_log
FOR EACH ROW EXECUTE FUNCTION audit.reject_audit_mutation();
