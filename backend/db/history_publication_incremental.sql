-- Explicitly selected public works; project and sibling results stay private.
BEGIN;
CREATE TABLE IF NOT EXISTS operation.published_work (
 id uuid PRIMARY KEY,
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 operation_id uuid NOT NULL REFERENCES operation.operation(id),
 output_id uuid NOT NULL REFERENCES operation.operation_output(id),
 media_asset_id uuid NOT NULL REFERENCES media.media_asset(id),
 title text NOT NULL CHECK (char_length(title) BETWEEN 1 AND 120),
 description text NOT NULL DEFAULT '' CHECK (char_length(description)<=2000),
 prompt text,
 created_at timestamptz NOT NULL DEFAULT now(),
 withdrawn_at timestamptz,
 UNIQUE(actor_id,output_id)
);
CREATE TABLE IF NOT EXISTS operation.history_publication_receipt (
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 request_key uuid NOT NULL,
 request_hash text NOT NULL,
 response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,org_id,scope_id,request_key)
);
GRANT SELECT,INSERT ON operation.published_work,operation.history_publication_receipt TO glowloom_app;
COMMIT;
