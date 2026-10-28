-- Personal views never alter operation execution or shared media ownership.
BEGIN;
CREATE TABLE IF NOT EXISTS operation.personal_history_record (
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 operation_id uuid NOT NULL REFERENCES operation.operation(id),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 deleted_at timestamptz,
 PRIMARY KEY(actor_id,org_id,scope_id,operation_id)
);
CREATE TABLE IF NOT EXISTS operation.personal_history_output (
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 operation_id uuid NOT NULL REFERENCES operation.operation(id),
 output_id uuid NOT NULL REFERENCES operation.operation_output(id),
 revision bigint NOT NULL DEFAULT 0 CHECK(revision>=0),
 favorite boolean NOT NULL DEFAULT false,
 deleted_at timestamptz,
 PRIMARY KEY(actor_id,org_id,scope_id,operation_id,output_id)
);
CREATE INDEX IF NOT EXISTS personal_history_output_favorite_idx
 ON operation.personal_history_output(actor_id,org_id,scope_id,operation_id) WHERE favorite AND deleted_at IS NULL;
CREATE TABLE IF NOT EXISTS operation.personal_history_command_receipt (
 actor_id uuid NOT NULL,
 org_id uuid NOT NULL,
 scope_id uuid NOT NULL,
 request_id uuid NOT NULL,
 request_hash text NOT NULL,
 response jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,org_id,scope_id,request_id)
);
GRANT SELECT,INSERT,UPDATE ON operation.personal_history_record,operation.personal_history_output TO glowloom_app;
GRANT SELECT,INSERT ON operation.personal_history_command_receipt TO glowloom_app;
COMMIT;
