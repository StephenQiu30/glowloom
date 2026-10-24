-- Rehearse on an isolated copy, then run as schema owner in one transaction.
-- This installs capability only; documents require an explicit revision-fenced migration.
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';
CREATE TABLE IF NOT EXISTS canvas.collab_document (
 document_id uuid PRIMARY KEY REFERENCES canvas.document(id),
 epoch uuid NOT NULL,
 writer_term bigint NOT NULL DEFAULT 1 CHECK(writer_term>0),
 commit_seq bigint NOT NULL DEFAULT 0 CHECK(commit_seq>=0),
 content_revision bigint NOT NULL CHECK(content_revision>0),
 state bytea NOT NULL CHECK(octet_length(state)<=8388608),
 graph_hash text NOT NULL CHECK(length(graph_hash)=64),
 model_version integer NOT NULL DEFAULT 1 CHECK(model_version=1),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 updated_at timestamptz NOT NULL DEFAULT clock_timestamp()
);
CREATE TABLE IF NOT EXISTS canvas.collab_update (
 document_id uuid NOT NULL REFERENCES canvas.collab_document(document_id),
 epoch uuid NOT NULL,
 commit_seq bigint NOT NULL CHECK(commit_seq>0),
 content_revision bigint NOT NULL,
 actor_id uuid NOT NULL,
 session_id uuid NOT NULL,
 update_id uuid NOT NULL,
 payload_hash text NOT NULL CHECK(length(payload_hash)=64),
 update_bytes bytea NOT NULL CHECK(octet_length(update_bytes)<=786432),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(document_id,epoch,commit_seq)
);
CREATE TABLE IF NOT EXISTS canvas.collab_receipt (
 document_id uuid NOT NULL REFERENCES canvas.document(id),
 epoch uuid NOT NULL,
 actor_id uuid NOT NULL,
 session_id uuid NOT NULL,
 update_id uuid NOT NULL,
 request_hash text NOT NULL CHECK(length(request_hash)=64),
 response_body jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(document_id,epoch,actor_id,session_id,update_id)
);
GRANT SELECT,INSERT,UPDATE ON canvas.collab_document TO glowloom_app;
GRANT SELECT,INSERT ON canvas.collab_update,canvas.collab_receipt TO glowloom_app;

CREATE OR REPLACE FUNCTION canvas.guard_collab_writer() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE target uuid;
BEGIN
 IF TG_TABLE_NAME='document' THEN target:=OLD.id;
 ELSIF TG_OP='DELETE' THEN target:=OLD.document_id;
 ELSE target:=NEW.document_id;
 END IF;
 -- Serialize migration with every old graph writer, including direct node jobs.
 PERFORM 1 FROM canvas.document WHERE id=target FOR UPDATE;
 IF EXISTS(SELECT 1 FROM canvas.collab_document WHERE document_id=target)
    AND current_setting('glowloom.yjs_document_id',true) IS DISTINCT FROM target::text THEN
  RAISE EXCEPTION 'canvas requires the Yjs acceptance path' USING ERRCODE='42501';
 END IF;
 IF TG_OP='DELETE' THEN RETURN OLD; END IF;
 RETURN NEW;
END $$;
CREATE OR REPLACE TRIGGER canvas_collab_document_writer BEFORE UPDATE OR DELETE ON canvas.document FOR EACH ROW EXECUTE FUNCTION canvas.guard_collab_writer();
CREATE OR REPLACE TRIGGER canvas_collab_node_writer BEFORE INSERT OR UPDATE OR DELETE ON canvas.node FOR EACH ROW EXECUTE FUNCTION canvas.guard_collab_writer();
CREATE OR REPLACE TRIGGER canvas_collab_edge_writer BEFORE INSERT OR UPDATE OR DELETE ON canvas.edge FOR EACH ROW EXECUTE FUNCTION canvas.guard_collab_writer();

CREATE TABLE IF NOT EXISTS canvas.collab_version (
 id uuid PRIMARY KEY, document_id uuid NOT NULL REFERENCES canvas.document(id),
 epoch uuid NOT NULL, revision bigint NOT NULL, document_json jsonb NOT NULL,
 writer_term bigint NOT NULL, commit_seq bigint NOT NULL, state bytea NOT NULL CHECK(octet_length(state)<=8388608), graph_hash text NOT NULL,
 actor_id uuid NOT NULL, reason text NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(document_id,revision), CHECK(octet_length(document_json::text)<=16777216)
);
CREATE TABLE IF NOT EXISTS canvas.collab_control_receipt (
 document_id uuid NOT NULL REFERENCES canvas.document(id), actor_id uuid NOT NULL,
 session_id uuid NOT NULL, command_id uuid NOT NULL, request_hash text NOT NULL,
 response_body jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 PRIMARY KEY(document_id,actor_id,session_id,command_id)
);
GRANT SELECT,INSERT ON canvas.collab_version,canvas.collab_control_receipt TO glowloom_app;
