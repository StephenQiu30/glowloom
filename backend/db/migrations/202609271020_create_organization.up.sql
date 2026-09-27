CREATE SCHEMA workspace;

CREATE TABLE workspace.organization (
  id          uuid PRIMARY KEY,
  name        text NOT NULL,
  status      text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(),
  is_delete   boolean NOT NULL DEFAULT false
);
