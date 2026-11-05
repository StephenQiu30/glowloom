-- Additive conversation data only. Apply offline; never replay service initialization.
BEGIN;
ALTER TABLE agent.service_runs ADD COLUMN IF NOT EXISTS input jsonb;
ALTER TABLE agent.service_runs ADD COLUMN IF NOT EXISTS retry_of_run_id uuid REFERENCES agent.service_runs(id);
ALTER TABLE agent.service_sessions ADD COLUMN IF NOT EXISTS last_input_at timestamptz;
CREATE INDEX IF NOT EXISTS service_runs_history ON agent.service_runs(session_id,accepted_revision DESC,id);
CREATE INDEX IF NOT EXISTS service_sessions_input ON agent.service_sessions(actor_id,project_id,last_input_at DESC,id);
CREATE TABLE IF NOT EXISTS agent.public_messages (
 session_id text NOT NULL REFERENCES agent.runtime_sessions(session_id),
 run_id uuid NOT NULL,
 commit_seq bigint NOT NULL,
 event_ordinal integer NOT NULL,
 part_index integer NOT NULL,
 message_id uuid NOT NULL,
 conversation_id bigint NOT NULL,
 sequence text NOT NULL,
 kind text NOT NULL,
 content text NOT NULL,
 PRIMARY KEY(session_id,commit_seq,event_ordinal,part_index)
);
CREATE INDEX IF NOT EXISTS public_messages_history ON agent.public_messages(run_id,commit_seq DESC,event_ordinal DESC,part_index DESC);
CREATE INDEX IF NOT EXISTS public_messages_identity ON agent.public_messages(run_id,message_id,part_index);
REVOKE ALL ON agent.public_messages FROM PUBLIC;
GRANT SELECT,INSERT ON agent.public_messages TO glowloom_agent;
COMMIT;
