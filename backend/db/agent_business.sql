-- Offline, reviewed business authority migration. Neither service runs DDL at startup.
-- No references to agent runtime tables. Apply as migration administrator.
CREATE SCHEMA IF NOT EXISTS agent_business;
CREATE TABLE IF NOT EXISTS agent_business.run_authorization (
 root_run_id uuid PRIMARY KEY, session_id uuid NOT NULL, submission_id uuid NOT NULL,
 request_id uuid NOT NULL, content_hash text NOT NULL, scope jsonb NOT NULL,
 budget_micros bigint NOT NULL CHECK(budget_micros>=0), reserved_micros bigint NOT NULL DEFAULT 0 CHECK(reserved_micros>=0),
 authorization_revision bigint NOT NULL DEFAULT 1 CHECK(authorization_revision>0),owner_epoch bigint NOT NULL DEFAULT 0,
 barrier boolean NOT NULL DEFAULT false, terminal boolean NOT NULL DEFAULT false,
 current_decision_id uuid, current_proposal_id uuid, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(session_id,request_id)
);
CREATE TABLE IF NOT EXISTS agent_business.execution_grant (
 grant_id uuid PRIMARY KEY,root_run_id uuid NOT NULL REFERENCES agent_business.run_authorization(root_run_id),
 submission_id uuid NOT NULL,owner_epoch bigint NOT NULL,authorization_revision bigint NOT NULL,expires_at timestamptz NOT NULL,
 UNIQUE(root_run_id,submission_id,owner_epoch,authorization_revision)
);
CREATE TABLE IF NOT EXISTS agent_business.control_decision (
 decision_id uuid PRIMARY KEY, root_run_id uuid NOT NULL REFERENCES agent_business.run_authorization(root_run_id),
 request_id uuid NOT NULL,input_hash text NOT NULL,authorization_revision bigint NOT NULL,
 action text NOT NULL CHECK(action IN ('approve','reject','pause','resume','stop')),body jsonb NOT NULL,
 delivery_state text NOT NULL DEFAULT 'pending' CHECK(delivery_state IN ('pending','leased','applied','superseded')),
 attempts integer NOT NULL DEFAULT 0,next_attempt_at timestamptz NOT NULL DEFAULT now(),lease_until timestamptz,
 applied_receipt jsonb,UNIQUE(root_run_id,request_id),UNIQUE(root_run_id,authorization_revision)
);
CREATE INDEX IF NOT EXISTS agent_business_decision_delivery ON agent_business.control_decision(next_attempt_at) WHERE delivery_state IN ('pending','leased');
CREATE TABLE IF NOT EXISTS agent_business.quiesced_receipt (
 decision_id uuid PRIMARY KEY REFERENCES agent_business.control_decision(decision_id),receipt_id uuid NOT NULL UNIQUE,
 root_run_id uuid NOT NULL REFERENCES agent_business.run_authorization(root_run_id),continuation_id uuid NOT NULL,
 owner_epoch bigint NOT NULL,authorization_revision bigint NOT NULL,grant_id uuid NOT NULL REFERENCES agent_business.execution_grant(grant_id),
 UNIQUE(root_run_id,continuation_id)
);
CREATE TABLE IF NOT EXISTS agent_business.proposal (
 proposal_id uuid PRIMARY KEY,root_run_id uuid NOT NULL REFERENCES agent_business.run_authorization(root_run_id),
 request_id uuid NOT NULL,input_hash text NOT NULL,candidate_hash text NOT NULL,body jsonb NOT NULL,
 authorization_revision bigint NOT NULL, UNIQUE(root_run_id,request_id)
);
CREATE TABLE IF NOT EXISTS agent_business.call_receipt (
 receipt_id uuid PRIMARY KEY,root_run_id uuid NOT NULL REFERENCES agent_business.run_authorization(root_run_id),submission_id uuid NOT NULL,
 task_id text NOT NULL,call_id text NOT NULL,checkpoint_id text NOT NULL,attempt bigint NOT NULL CHECK(attempt>0),contract_version text NOT NULL,
 input_hash text NOT NULL,kind text NOT NULL,name text NOT NULL,max_micros bigint NOT NULL CHECK(max_micros>=0),
 status text NOT NULL CHECK(status IN ('in_flight','completed','failed','unknown')),result jsonb NOT NULL DEFAULT '{}',cost_micros bigint NOT NULL DEFAULT 0,
 created_at timestamptz NOT NULL DEFAULT now(),UNIQUE(root_run_id,submission_id,task_id,call_id,checkpoint_id,attempt,contract_version)
);
-- A source publication retains its original call/grant identity across I/O and restarts.
CREATE TABLE IF NOT EXISTS agent_business.source_authorization (
 receipt_id uuid PRIMARY KEY REFERENCES agent_business.call_receipt(receipt_id) ON DELETE CASCADE,
 grant_id uuid NOT NULL REFERENCES agent_business.execution_grant(grant_id),
 source_hash text NOT NULL CHECK(source_hash ~ '^[a-f0-9]{64}$')
);
CREATE TABLE IF NOT EXISTS agent_business.skill_version (
 skill_id uuid NOT NULL,version text NOT NULL,resource_hash text NOT NULL,owner_actor_id uuid NOT NULL,org_id uuid NOT NULL,scope_id uuid NOT NULL,
 name text NOT NULL,description text NOT NULL,body text NOT NULL,resources jsonb NOT NULL DEFAULT '{}',enabled boolean NOT NULL DEFAULT true,
 PRIMARY KEY(skill_id,version),CHECK(length(body)<=262144)
);
REVOKE ALL ON SCHEMA agent_business FROM PUBLIC;
REVOKE ALL ON ALL TABLES IN SCHEMA agent_business FROM PUBLIC;
-- Application roles must already exist; this fragment never creates login credentials.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='glowloom_app') THEN
  GRANT USAGE ON SCHEMA agent_business TO glowloom_app;
  GRANT SELECT,INSERT,UPDATE ON ALL TABLES IN SCHEMA agent_business TO glowloom_app;
 END IF;
 IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname='glowloom_agent') THEN
  REVOKE ALL ON SCHEMA agent_business FROM glowloom_agent;
  REVOKE ALL ON ALL TABLES IN SCHEMA agent_business FROM glowloom_agent;
 END IF;
END $$;
-- Durable import identity is separate from immutable resource identity.
ALTER TABLE agent_business.skill_version ADD COLUMN IF NOT EXISTS request_id uuid;
CREATE UNIQUE INDEX IF NOT EXISTS agent_business_skill_import_request ON agent_business.skill_version(owner_actor_id,org_id,scope_id,request_id);
-- Immutable authorized Skill pins are business scope, not Harness execution state.
ALTER TABLE agent_business.run_authorization ADD COLUMN IF NOT EXISTS skill_pins jsonb NOT NULL DEFAULT '[]';
-- Go freezes the admitted model selection independently of Node's manifest.
ALTER TABLE agent_business.run_authorization ADD COLUMN IF NOT EXISTS model_selection jsonb NOT NULL DEFAULT '{}';
