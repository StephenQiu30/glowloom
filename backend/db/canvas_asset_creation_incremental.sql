-- Additive asset-entry creation receipt. Never rewrites existing project/canvas data.
BEGIN;
CREATE TABLE IF NOT EXISTS canvas.asset_creation_receipt (
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 scope_id uuid NOT NULL REFERENCES workspace.creation_scope(id),
 idem_key uuid NOT NULL,
 request_hash text NOT NULL CHECK(length(request_hash)=64),
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 document_id uuid NOT NULL REFERENCES canvas.document(id),
 response_body jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,idem_key)
);
GRANT SELECT,INSERT ON canvas.asset_creation_receipt TO glowloom_app;
COMMIT;
