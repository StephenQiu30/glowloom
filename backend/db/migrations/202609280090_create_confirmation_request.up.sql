CREATE TABLE operation.confirmation_request (
  org_id       uuid NOT NULL,
  actor_id     uuid NOT NULL,
  request_id   uuid NOT NULL,
  fingerprint  bytea NOT NULL CHECK (octet_length(fingerprint) = 32),
  outcome      jsonb NOT NULL CHECK (jsonb_typeof(outcome) = 'object'),
  create_time  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (org_id, actor_id, request_id)
);

GRANT SELECT, INSERT ON TABLE operation.confirmation_request TO lanverse_app;
