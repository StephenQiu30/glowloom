-- Additive editor persistence. Apply to an existing database only after review.
BEGIN;
CREATE TABLE workspace.prompt_draft (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 revision bigint NOT NULL CHECK(revision BETWEEN 1 AND 2147483647),
 draft jsonb NOT NULL CHECK(jsonb_typeof(draft)='object' AND octet_length(draft::text)<=1048576),
 draft_hash text NOT NULL CHECK(draft_hash ~ '^[a-f0-9]{64}$'),
 input_hash text NOT NULL CHECK(input_hash ~ '^[a-f0-9]{64}$'),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX prompt_draft_owner ON workspace.prompt_draft(project_id,actor_id,id);
CREATE TABLE workspace.prompt_draft_receipt (
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 request_key uuid NOT NULL,
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 request_hash text NOT NULL,
 response jsonb NOT NULL,
 PRIMARY KEY(actor_id,request_key)
);
CREATE TABLE operation.prompt_submission_slot (
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 owner_key text NOT NULL,
 epoch bigint NOT NULL DEFAULT 0 CHECK(epoch>=0),
 submission_id uuid,
 PRIMARY KEY(actor_id,project_id,owner_key)
);
CREATE TABLE operation.prompt_submission (
 id uuid PRIMARY KEY,
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 owner_key text NOT NULL,
 epoch bigint NOT NULL CHECK(epoch>0),
 operation_id uuid NOT NULL UNIQUE REFERENCES operation.operation(id),
 confirm_key uuid NOT NULL UNIQUE,
 input_source jsonb NOT NULL,
 snapshot jsonb NOT NULL,
 quote jsonb NOT NULL,
 state text NOT NULL CHECK(state IN ('quoted','accepted','closed')),
 resolution text,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(actor_id,project_id,owner_key,epoch)
);
CREATE TABLE operation.prompt_resolution_receipt (
 actor_id uuid NOT NULL,
 request_key uuid NOT NULL,
 request_hash text NOT NULL,
 response jsonb NOT NULL,
 PRIMARY KEY(actor_id,request_key)
);
GRANT SELECT,INSERT,UPDATE ON workspace.prompt_draft,operation.prompt_submission_slot,operation.prompt_submission TO lanverse_app;
GRANT SELECT,INSERT ON workspace.prompt_draft_receipt,operation.prompt_resolution_receipt TO lanverse_app;
COMMIT;
