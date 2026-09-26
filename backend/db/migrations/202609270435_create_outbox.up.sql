CREATE SCHEMA infra;

CREATE TABLE infra.outbox (
  id            uuid NOT NULL,
  topic         text NOT NULL,
  partition_key text NOT NULL,
  payload       jsonb NOT NULL,
  headers       jsonb NOT NULL DEFAULT '{}',
  create_time   timestamptz NOT NULL DEFAULT now(),
  published_at  timestamptz,
  update_time   timestamptz NOT NULL DEFAULT now(),
  is_delete     boolean NOT NULL DEFAULT false,
  PRIMARY KEY (id, create_time)
) PARTITION BY RANGE (create_time);

CREATE INDEX ix_outbox_pending ON infra.outbox (create_time) WHERE published_at IS NULL;

-- Keep the current UTC month and three future months ready for writes.
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
      'CREATE TABLE infra.%I PARTITION OF infra.outbox FOR VALUES FROM (%L) TO (%L)',
      'outbox_' || to_char(month_start AT TIME ZONE 'UTC', 'YYYYMM'),
      month_start,
      month_end
    );
  END LOOP;
END $$;

-- The partition maintenance job is not installed yet. Keep writes available
-- outside the four-month window; the job must move these rows before attach.
CREATE TABLE infra.outbox_default PARTITION OF infra.outbox DEFAULT;
