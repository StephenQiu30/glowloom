CREATE INDEX ix_outbox_published_retention
  ON infra.outbox (published_at, create_time, id)
  WHERE published_at IS NOT NULL;
