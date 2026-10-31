BEGIN;
-- Private canvas assistant history; generation drafts and tasks keep their owners.
CREATE TABLE IF NOT EXISTS canvas.assistant_session (
 id uuid PRIMARY KEY,
 canvas_id uuid NOT NULL REFERENCES canvas.document(id),
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 title text NOT NULL CHECK (length(btrim(title)) BETWEEN 1 AND 128),
 revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
 turns jsonb NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(turns)='array' AND jsonb_array_length(turns)<=64 AND octet_length(turns::text)<=65536),
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS assistant_session_owner ON canvas.assistant_session(canvas_id,actor_id,updated_at DESC,id);
GRANT SELECT,INSERT,UPDATE ON canvas.assistant_session TO glowloom_app;

COMMIT;
