CREATE SCHEMA canvas;
CREATE TABLE canvas.document (
  id uuid PRIMARY KEY, project_id uuid NOT NULL REFERENCES workspace.project(id),
  name text NOT NULL, scope jsonb NOT NULL DEFAULT '{}', revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
  viewport jsonb NOT NULL DEFAULT '{}', create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(), is_delete boolean NOT NULL DEFAULT false
);
CREATE INDEX ix_canvas_document_project ON canvas.document(project_id) WHERE NOT is_delete;
CREATE TABLE canvas.node (
  id uuid PRIMARY KEY, document_id uuid NOT NULL REFERENCES canvas.document(id),
  node_type text NOT NULL, node_action text CHECK (node_action IN ('resource','generate','edit')),
  ref_type text, ref_id uuid, config jsonb NOT NULL DEFAULT '{}',
  x double precision NOT NULL, y double precision NOT NULL, width double precision, height double precision,
  parent_id uuid, z_index integer NOT NULL DEFAULT 0, last_operation_id uuid,
  update_time timestamptz NOT NULL DEFAULT now(), create_time timestamptz NOT NULL DEFAULT now(),
  is_delete boolean NOT NULL DEFAULT false, UNIQUE(document_id,id)
);
CREATE INDEX ix_canvas_node_doc ON canvas.node(document_id);
CREATE TABLE canvas.edge (
  id uuid PRIMARY KEY, document_id uuid NOT NULL REFERENCES canvas.document(id),
  edge_type text NOT NULL CHECK (edge_type IN ('input','reference','promote','annotation')),
  source_node_id uuid NOT NULL, target_node_id uuid NOT NULL, role text, binding jsonb,
  create_time timestamptz NOT NULL DEFAULT now(), update_time timestamptz NOT NULL DEFAULT now(),
  is_delete boolean NOT NULL DEFAULT false,
  FOREIGN KEY(document_id,source_node_id) REFERENCES canvas.node(document_id,id),
  FOREIGN KEY(document_id,target_node_id) REFERENCES canvas.node(document_id,id)
);
CREATE TABLE canvas.command_log (
  id uuid PRIMARY KEY, document_id uuid NOT NULL REFERENCES canvas.document(id), revision integer NOT NULL,
  commands jsonb NOT NULL, create_by uuid, create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(), is_delete boolean NOT NULL DEFAULT false,
  CONSTRAINT uq_canvas_command_rev UNIQUE(document_id,revision)
);
CREATE TABLE infra.idempotency_record (
  id uuid PRIMARY KEY, actor_id uuid NOT NULL, idem_key text NOT NULL, request_hash text NOT NULL,
  status_code integer, response_body jsonb, create_time timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL, update_time timestamptz NOT NULL DEFAULT now(),
  is_delete boolean NOT NULL DEFAULT false, CONSTRAINT uq_idempotency_record_natural UNIQUE(actor_id,idem_key)
);
GRANT USAGE ON SCHEMA canvas TO lanverse_app;
GRANT SELECT,INSERT,UPDATE ON ALL TABLES IN SCHEMA canvas TO lanverse_app;
GRANT SELECT,INSERT,UPDATE,DELETE ON infra.idempotency_record TO lanverse_app;
