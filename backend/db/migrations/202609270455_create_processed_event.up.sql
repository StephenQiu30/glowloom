CREATE TABLE infra.processed_event (
  id          uuid PRIMARY KEY,
  consumer    text NOT NULL,
  event_id    uuid NOT NULL,
  create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(),
  is_delete   boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_processed_event_natural UNIQUE (consumer, event_id)
);
