CREATE TABLE operation.batch_launch (
  operation_id uuid PRIMARY KEY REFERENCES operation.operation(id),
  batch_id     uuid NOT NULL,
  project_id   uuid NOT NULL,
  provider_id  uuid REFERENCES catalog.provider(id),
  state        text NOT NULL CHECK (state IN ('active', 'done')),
  acquired_at  timestamptz NOT NULL DEFAULT now(),
  released_at  timestamptz,
  CONSTRAINT fk_batch_launch_batch_project FOREIGN KEY (project_id, batch_id)
    REFERENCES operation.batch(project_id, id),
  CONSTRAINT ck_batch_launch_release CHECK (
    (state = 'active' AND released_at IS NULL) OR
    (state = 'done' AND released_at IS NOT NULL)
  )
);

CREATE INDEX ix_batch_launch_active_project
  ON operation.batch_launch (project_id)
  WHERE state = 'active';

CREATE INDEX ix_batch_launch_active_provider
  ON operation.batch_launch (provider_id)
  WHERE state = 'active' AND provider_id IS NOT NULL;

GRANT SELECT, INSERT, UPDATE (state, released_at)
  ON TABLE operation.batch_launch TO lanverse_app;

GRANT UPDATE (succeeded_count, failed_count, unknown_count, paused_reason)
  ON TABLE operation.batch TO lanverse_app;
