-- Private, bounded Agent sessions. No supplier credentials or temporary URLs.
CREATE SCHEMA IF NOT EXISTS agent;
CREATE TABLE IF NOT EXISTS agent.session (
 id uuid PRIMARY KEY,
 project_id uuid NOT NULL REFERENCES workspace.project(id),
 actor_id uuid NOT NULL REFERENCES identity."user"(id),
 request_id uuid NOT NULL,
 title text NOT NULL CHECK(length(title)>0 AND octet_length(title)<=240),
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 turns jsonb NOT NULL DEFAULT '[]'::jsonb CHECK(jsonb_typeof(turns)='array' AND jsonb_array_length(turns)<=20 AND octet_length(turns::text)<=1048576),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(project_id,actor_id,request_id)
);
CREATE INDEX IF NOT EXISTS agent_session_owner_updated ON agent.session(project_id,actor_id,updated_at DESC,id);
GRANT USAGE ON SCHEMA agent TO glowloom_app;
GRANT SELECT,INSERT,UPDATE ON agent.session TO glowloom_app;
