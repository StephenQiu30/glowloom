-- Current independent Agent admission metadata. Apply offline after agent_storage.sql.
BEGIN;
CREATE TABLE agent.service_sessions (
 id uuid PRIMARY KEY, actor_id uuid NOT NULL, org_id uuid NOT NULL, scope_id uuid NOT NULL,
 project_id uuid NOT NULL, canvas_id uuid, title text NOT NULL,
 request_id uuid NOT NULL, content_hash text NOT NULL,
 execution_revision bigint NOT NULL DEFAULT 0 CHECK(execution_revision>=0),
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(actor_id,project_id,request_id)
);
CREATE INDEX service_sessions_owner ON agent.service_sessions(actor_id,project_id,created_at DESC,id);
CREATE TABLE agent.service_runs (
 id uuid PRIMARY KEY, session_id uuid NOT NULL REFERENCES agent.service_sessions,
 request_id uuid NOT NULL, content_hash text NOT NULL, goal text NOT NULL, manifest jsonb NOT NULL,
 submission_id uuid NOT NULL, accepted_revision bigint NOT NULL,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(session_id,request_id)
);
CREATE TABLE agent.service_control_inbox (
 decision_id uuid PRIMARY KEY, root_run_id uuid NOT NULL REFERENCES agent.service_runs,
 authorization_revision bigint NOT NULL, content_hash text NOT NULL, input jsonb NOT NULL,
 receipt jsonb, created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 UNIQUE(root_run_id,authorization_revision)
);
CREATE INDEX service_control_pending ON agent.service_control_inbox(root_run_id,authorization_revision) WHERE receipt IS NULL;
CREATE INDEX runtime_events_run ON agent.runtime_events(session_id,(payload->>'root_run_id'),commit_seq,event_ordinal);
REVOKE ALL ON agent.service_sessions,agent.service_runs,agent.service_control_inbox FROM PUBLIC;
GRANT SELECT,INSERT,UPDATE ON agent.service_sessions,agent.service_control_inbox TO glowloom_agent;
GRANT SELECT,INSERT ON agent.service_runs TO glowloom_agent;
COMMIT;
