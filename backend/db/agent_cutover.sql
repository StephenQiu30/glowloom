-- One-time offline old-Agent/Temporal cutover; NOT an application migration.
-- Read agent_cutover.md first. Never run schema.sql on an existing database.
-- This script does not stop services, cancel tasks, settle charges or create
-- login credentials. Missing evidence, old work, unknown cost or schema drift
-- aborts the entire transaction before any old table is removed.
\set ON_ERROR_STOP on
\if :{?cutover_database}
\else
DO $proof$ BEGIN RAISE EXCEPTION 'required offline evidence parameter cutover_database is missing; nothing was changed'; END $proof$;
\endif
\if :{?clearance_as_of}
\else
DO $proof$ BEGIN RAISE EXCEPTION 'required offline evidence parameter clearance_as_of is missing; nothing was changed'; END $proof$;
\endif
\if :{?backup_sha256}
\else
\echo 'ERROR: required offline evidence parameter backup_sha256 is missing; nothing was changed'
\quit 3
\endif
\if :{?inventory_sha256}
\else
\echo 'ERROR: required offline evidence parameter inventory_sha256 is missing; nothing was changed'
\quit 3
\endif
\if :{?external_clearance_sha256}
\else
\echo 'ERROR: required offline evidence parameter external_clearance_sha256 is missing; nothing was changed'
\quit 3
\endif
\if :{?history_drop_authorization_sha256}
\else
\echo 'ERROR: required offline evidence parameter history_drop_authorization_sha256 is missing; nothing was changed'
\quit 3
\endif
\if :{?stopped_writes_sha256}
\else
\echo 'ERROR: required offline evidence parameter stopped_writes_sha256 is missing; nothing was changed'
\quit 3
\endif
\if :{?external_clearance_state}
\else
DO $proof$ BEGIN RAISE EXCEPTION 'required offline evidence parameter external_clearance_state is missing; nothing was changed'; END $proof$;
\endif

BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='5min';
SELECT set_config('glowloom.cutover.proof',jsonb_build_object(
 'database',:'cutover_database','clearance_as_of',:'clearance_as_of',
 'backup_sha256',:'backup_sha256','inventory_sha256',:'inventory_sha256',
 'external_clearance_sha256',:'external_clearance_sha256',
 'history_drop_authorization_sha256',:'history_drop_authorization_sha256',
 'stopped_writes_sha256',:'stopped_writes_sha256',
 'external_clearance_state',:'external_clearance_state')::text,true) AS proof \gset

DO $gate$
DECLARE
 proof jsonb := current_setting('glowloom.cutover.proof')::jsonb;
 key text;
 obj text;
 old_tables text[] := ARRAY['session','runtime_session','runtime_commit','harness_run','harness_candidate','harness_command','harness_approval','harness_admission'];
BEGIN
 IF proof->>'database' IS DISTINCT FROM current_database() THEN
  RAISE EXCEPTION 'cutover database does not match the reviewed evidence';
 END IF;
 FOREACH key IN ARRAY ARRAY['backup_sha256','inventory_sha256','external_clearance_sha256','history_drop_authorization_sha256','stopped_writes_sha256'] LOOP
  IF coalesce(proof->>key,'') !~ '^[a-f0-9]{64}$' OR proof->>key=repeat('0',64) THEN
   RAISE EXCEPTION 'missing/invalid independent evidence hash: %',key;
  END IF;
 END LOOP;
 IF proof->>'external_clearance_state' IS DISTINCT FROM 'all-old-executions-and-charges-closed' THEN
  RAISE EXCEPTION 'external execution/charge state is not explicitly cleared';
 END IF;
 IF (proof->>'clearance_as_of')::timestamptz < clock_timestamp()-interval '30 minutes'
 OR (proof->>'clearance_as_of')::timestamptz > clock_timestamp()+interval '1 minute' THEN
  RAISE EXCEPTION 'external/offline clearance must be reconfirmed within 30 minutes';
 END IF;
 IF EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND backend_type='client backend') THEN
  RAISE EXCEPTION 'offline cutover requires every other client connection to this database to be closed';
 END IF;
 PERFORM pg_advisory_xact_lock(741104,32);
 -- No partially applied/new installation is accepted; a second execution is
 -- explicitly rejected instead of becoming an idempotent mixed-engine path.
 IF to_regnamespace('agent_business') IS NOT NULL OR to_regclass('infra.task_work') IS NOT NULL
 OR to_regclass('operation.development_trial') IS NOT NULL
 OR EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='agent' AND c.relname IN ('runtime_sessions','runtime_records','runtime_events','service_sessions','service_runs'))
 OR EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='operation' AND table_name='batch' AND column_name='resumed_at') THEN
  RAISE EXCEPTION 'current/partial cutover objects already exist; repeat execution is forbidden';
 END IF;
 FOREACH obj IN ARRAY old_tables LOOP
  IF to_regclass('agent.'||obj) IS NULL THEN
   RAISE EXCEPTION 'required HEAD old schema object agent.% is missing; review deployment-specific schema',obj;
  END IF;
 END LOOP;
 IF EXISTS(SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
  WHERE n.nspname='agent' AND c.relkind IN ('r','p','v','m','f','S')
  AND NOT c.relname=ANY(old_tables||ARRAY['graph_checkpoint','runtime_session_lock_key_seq']))
 OR EXISTS(SELECT 1 FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='agent') THEN
  RAISE EXCEPTION 'unknown old agent relation/function; inventory and review it before cutover';
 END IF;
 IF EXISTS(SELECT 1 FROM pg_constraint c JOIN pg_class target ON target.oid=c.confrelid
  JOIN pg_namespace n ON n.oid=target.relnamespace
  WHERE c.contype='f' AND n.nspname='agent' AND c.conrelid NOT IN
   (SELECT oid FROM pg_class WHERE relnamespace=n.oid AND relname=ANY(old_tables||ARRAY['graph_checkpoint']))) THEN
  RAISE EXCEPTION 'external inbound FK references old agent data; no CASCADE/drop is allowed';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='glowloom_agent' AND rolcanlogin AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolbypassrls AND NOT rolreplication)
 OR NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='glowloom_app' AND NOT rolcanlogin AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolbypassrls AND NOT rolreplication)
 OR NOT EXISTS(SELECT 1 FROM pg_roles WHERE rolname='glowloom_agent_runtime' AND NOT rolcanlogin AND NOT rolsuper AND NOT rolcreatedb AND NOT rolcreaterole AND NOT rolbypassrls AND NOT rolreplication) THEN
  RAISE EXCEPTION 'pre-provision reviewed restricted glowloom_agent LOGIN and glowloom_app/old runtime NOLOGIN roles; this script never sets credentials';
 END IF;
 IF EXISTS(SELECT 1 FROM pg_auth_members WHERE member=(SELECT oid FROM pg_roles WHERE rolname='glowloom_agent')
  OR roleid=(SELECT oid FROM pg_roles WHERE rolname='glowloom_agent')
  OR member=(SELECT oid FROM pg_roles WHERE rolname='glowloom_agent_runtime'))
 OR has_database_privilege('glowloom_agent',current_database(),'CREATE')
 OR has_database_privilege('glowloom_agent_runtime',current_database(),'CREATE')
 OR EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='agent' AND nspowner IN (SELECT oid FROM pg_roles WHERE rolname IN ('glowloom_app','glowloom_agent','glowloom_agent_runtime'))) THEN
  RAISE EXCEPTION 'Agent roles have membership/ownership/create privileges inconsistent with isolation';
 END IF;
 IF EXISTS(SELECT 1 FROM pg_shdepend d
  WHERE d.dbid=(SELECT oid FROM pg_database WHERE datname=current_database())
  AND d.refclassid='pg_authid'::regclass AND d.refobjid=(SELECT oid FROM pg_roles WHERE rolname='glowloom_agent_runtime')
  AND NOT ((d.classid='pg_namespace'::regclass AND d.objid='agent'::regnamespace)
   OR (d.classid='pg_class'::regclass AND d.objid IN (SELECT oid FROM pg_class WHERE relnamespace='agent'::regnamespace)))) THEN
  RAISE EXCEPTION 'old runtime role has privileges outside old agent objects; review these explicitly instead of DROP OWNED';
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='operation.operation'::regclass AND conname='ck_operation_agent_session') THEN
  RAISE EXCEPTION 'old operation source/target constraints differ from reviewed HEAD';
 END IF;
END
$gate$;

-- Freeze every PG source read by the shared blocker query. Offline shutdown
-- remains mandatory; locks only close the inventory-to-DDL transaction race.
LOCK TABLE agent.session,agent.runtime_session,agent.runtime_commit,
 agent.harness_run,agent.harness_candidate,agent.harness_command,agent.harness_approval,agent.harness_admission,
 operation.operation,operation.batch,operation.provider_call,billing.reservation,
 catalog.model_validation,catalog.provider_check,media.package_job,media.purge_job,
 media.transfer_job,media.storage_work_intent,media.media_asset,mediatool.depth_job,
 mediatool.export_job,mediatool.transcription_job,script.import_state,script.command_state,
 workspace.project_copy_job,workspace.project_retention_job,subject.voice_job,
 identity.directory_export,infra.outbox IN ACCESS EXCLUSIVE MODE;

\ir agent_cutover_blockers.sql
SELECT set_config('glowloom.cutover.blockers',:'cutover_blocker_inventory',true) AS blockers \gset
SELECT * FROM jsonb_to_recordset(current_setting('glowloom.cutover.blockers')::jsonb)
 AS b(check_name text,blocked_rows bigint,reason text) ORDER BY check_name;
DO $gate$
DECLARE blockers text;
BEGIN
 SELECT string_agg(check_name||'='||blocked_rows::text,', ' ORDER BY check_name)
 INTO blockers FROM jsonb_to_recordset(current_setting('glowloom.cutover.blockers')::jsonb)
  AS b(check_name text,blocked_rows bigint,reason text) WHERE blocked_rows<>0;
 IF blockers IS NOT NULL THEN
  RAISE EXCEPTION 'cutover blocked by old work/data/charges: %. Reconcile through reviewed business procedures; evidence hashes cannot bypass PG blockers',blockers;
 END IF;
END
$gate$;

-- Backed-up old product history is deliberately removed, never translated or
-- exposed through a compatibility reader. Exact names + RESTRICT preserve all
-- outside business data/dependencies. Identity sequence is table-owned.
DROP TABLE agent.harness_admission RESTRICT;
DROP TABLE agent.harness_approval RESTRICT;
DROP TABLE agent.harness_command RESTRICT;
DROP TABLE agent.harness_candidate RESTRICT;
DROP TABLE agent.harness_run RESTRICT;
DROP TABLE agent.runtime_commit RESTRICT;
DROP TABLE agent.runtime_session RESTRICT;
DROP TABLE IF EXISTS agent.graph_checkpoint RESTRICT;
DROP TABLE agent.session RESTRICT;
REVOKE ALL ON SCHEMA agent FROM PUBLIC,glowloom_app,glowloom_agent_runtime;
-- Do not DROP OWNED or DROP the cluster-wide role: its other-database grants
-- are outside this migration. It is NOLOGIN and loses all current-db objects.

-- Remove the old operation carve-out; retain every business/ledger row. Any
-- agent_session target was already a hard blocker, even when terminal.
ALTER TABLE operation.operation DROP CONSTRAINT ck_operation_agent_session;
ALTER TABLE operation.operation DROP CONSTRAINT ck_operation_upload;
ALTER TABLE operation.operation ADD CONSTRAINT ck_operation_upload
 CHECK (origin='upload' OR (region IS NOT NULL AND model_profile_version_id IS NOT NULL));
ALTER TABLE operation.operation DROP CONSTRAINT operation_target_type_check;
ALTER TABLE operation.operation ADD CONSTRAINT operation_target_type_check
 CHECK (target_type IN ('shot_frame','shot_take','reference_slot','dialogue_audio','voice_preview','episode_split','episode_parse','bible_extract','scene_storyboard','free'));

-- Frozen target fragments follow. BEGIN/COMMIT wrappers are removed so the
-- whole cutover is ONE transaction. Source hashes are review anchors; do not
-- replace these with \ir transaction-bearing fragments (that would commit the
-- destructive prefix early). No old data/task/execution state is copied.

-- BEGIN TARGET agent_storage.sql sha256=31b8ba1defea7fbcd8f24027ea93804e8baf1440339bab074543bb41f326d0b4
-- Offline migration. A deployment administrator provisions the restricted login
-- glowloom_agent beforehand; the service never creates roles, schemas or tables.

CREATE SCHEMA IF NOT EXISTS agent;
CREATE TABLE agent.runtime_sessions (
    session_id text PRIMARY KEY,
    owner_id text NOT NULL,
    owner_epoch bigint NOT NULL CHECK (owner_epoch > 0),
    lease_until timestamptz NOT NULL,
    last_id bigint NOT NULL DEFAULT 1 CHECK (last_id BETWEEN 1 AND 9007199254740991),
    commit_seq bigint NOT NULL DEFAULT 0 CHECK (commit_seq BETWEEN 0 AND 9007199254740991)
);
-- JSON encoded text preserves JS strings including lone surrogates and NUL.
-- Indexed identity columns store JSON encoded scalar strings for the same reason.
CREATE TABLE agent.runtime_records (
    session_id text NOT NULL REFERENCES agent.runtime_sessions,
    id bigint NOT NULL CHECK (id BETWEEN 1 AND 9007199254740991),
    record_type text NOT NULL CHECK (record_type IN ('conversation','entry','task','submission','document')),
    body text NOT NULL,
    commit_seq bigint NOT NULL,
    conversation_id bigint,
    owner_conversation_id bigint,
    owner_task_id bigint,
    parent_id bigint,
    parent_at bigint,
    head_id bigint,
    kind_key text,
    status text,
    background boolean,
    abort_requested boolean,
    request_key text,
    scope_key text,
    address_key text,
    created_at_seq bigint,
    retired_at_seq bigint,
    PRIMARY KEY (session_id, id)
);
CREATE INDEX runtime_records_scan ON agent.runtime_records(session_id, record_type, id);
CREATE INDEX runtime_records_conversation ON agent.runtime_records(session_id, record_type, conversation_id, id);
CREATE INDEX runtime_records_owner_task ON agent.runtime_records(session_id, owner_task_id, id) WHERE owner_task_id IS NOT NULL;
-- Hash exclusion rechecks exact equality and avoids btree's key-size limit for
-- arbitrary SDK strings. No digest is treated as a durable identity.
ALTER TABLE agent.runtime_records ADD CONSTRAINT runtime_submission_request
  EXCLUDE USING hash ((ARRAY[session_id, conversation_id::text, request_key]) WITH =) WHERE (request_key IS NOT NULL);
CREATE INDEX runtime_request_lookup ON agent.runtime_records USING hash(request_key) WHERE request_key IS NOT NULL;
ALTER TABLE agent.runtime_records ADD CONSTRAINT runtime_live_document
  EXCLUDE USING hash ((ARRAY[session_id,address_key]) WITH =) WHERE (address_key IS NOT NULL AND retired_at_seq IS NULL);
CREATE INDEX runtime_document_address ON agent.runtime_records USING hash(address_key) WHERE address_key IS NOT NULL;
CREATE INDEX runtime_document_scope ON agent.runtime_records(session_id,scope_key,id) WHERE scope_key IS NOT NULL;
CREATE TABLE agent.runtime_document_versions (
    session_id text NOT NULL,
    document_id bigint NOT NULL,
    commit_seq bigint NOT NULL,
    version integer NOT NULL,
    content text NOT NULL,
    deltas_since_base integer NOT NULL,
    PRIMARY KEY(session_id,document_id,commit_seq),
    FOREIGN KEY(session_id,document_id) REFERENCES agent.runtime_records
);
CREATE TABLE agent.runtime_events (
    session_id text NOT NULL REFERENCES agent.runtime_sessions,
    commit_seq bigint NOT NULL,
    event_ordinal integer NOT NULL CHECK (event_ordinal > 0),
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    PRIMARY KEY (session_id,commit_seq,event_ordinal)
);
REVOKE ALL ON agent.runtime_sessions, agent.runtime_records, agent.runtime_document_versions, agent.runtime_events FROM PUBLIC;
GRANT USAGE ON SCHEMA agent TO glowloom_agent;
GRANT SELECT,INSERT,UPDATE ON agent.runtime_sessions, agent.runtime_records TO glowloom_agent;
GRANT SELECT,INSERT,DELETE ON agent.runtime_document_versions, agent.runtime_events TO glowloom_agent;
-- END TARGET agent_storage.sql

-- BEGIN TARGET agent_service.sql sha256=77f2a1a6db632b935c6d405fbad2d8cbd5338ae541ad82e5cf2a80e5ca48107c
-- Current independent Agent admission metadata. Apply offline after agent_storage.sql.

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
-- END TARGET agent_service.sql

-- BEGIN TARGET agent_business.sql sha256=d65ed861c146dc37de931de4ceba05ba9b6edd836d00dbef34ab06172ea9dd5e
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
-- END TARGET agent_business.sql

-- BEGIN TARGET task_runtime.sql sha256=f9d78d2dbba25daa6b52db7088c10ba1e3744c104add2ff624cd365592115987
-- Install in an empty or explicitly migrated business database.
CREATE TABLE infra.task_work (
    kind text NOT NULL,
    business_id text NOT NULL,
    next_run_at timestamptz NOT NULL,
    owner text,
    epoch bigint NOT NULL DEFAULT 0,
    lease_until timestamptz,
    attempt integer NOT NULL DEFAULT 0,
    reference jsonb NOT NULL DEFAULT 'null',
    failures integer NOT NULL DEFAULT 0,
    failed_at timestamptz,
    stop_requested_at timestamptz,
    completed_at timestamptz,
    PRIMARY KEY (kind, business_id),
    CHECK (kind <> '' AND business_id <> ''),
    CHECK ((owner IS NULL) = (lease_until IS NULL))
);
CREATE INDEX task_work_due ON infra.task_work (kind, next_run_at) WHERE completed_at IS NULL AND failed_at IS NULL;

-- Exactly one cursor for each periodic business sweep. Restarting a process
-- never resets its due time. The worker checks existing expiries on first claim.
INSERT INTO infra.task_work (kind,business_id,next_run_at) VALUES
 ('maintenance.partitions','periodic',now()),
 ('maintenance.outbox','periodic',now()),
 ('maintenance.inbox','periodic',now()),
 -- Content deletion remains disabled until an operator explicitly enables it.
 ('workspace.retention','periodic','infinity'),
 ('operation.quote_expiry','periodic',now());

-- Administrator trials previously kept their owner-scoped state in external
-- execution history. This is the business record for new local trial requests.
CREATE TABLE operation.development_trial (
 id text PRIMARY KEY,
 input jsonb NOT NULL,
 state jsonb NOT NULL,
 phase text NOT NULL CHECK(phase IN ('ready','dispatching','poll','done')),
 generation bigint NOT NULL DEFAULT 1,
 polls integer NOT NULL DEFAULT 0,
 updated_at timestamptz NOT NULL DEFAULT now()
);

GRANT SELECT, INSERT, UPDATE ON infra.task_work, operation.development_trial TO glowloom_app;
-- END TARGET task_runtime.sql

-- BEGIN TARGET operation_task.sql sha256=e13030a080a2c355ca1add2818cbada06839445d4ee92fec993766bad794eac1
-- A resumed batch acknowledges failures observed before the operator reopened admission.
ALTER TABLE operation.batch ADD COLUMN resumed_at timestamptz;
GRANT SELECT(resumed_at), UPDATE(resumed_at) ON operation.batch TO glowloom_app;
-- END TARGET operation_task.sql

-- BEGIN TARGET agent_conversation.sql sha256=dbd27392ae94e2f8fb9b56419b9a67d4606a0707f66a82e5a3303e22d349e383
-- Additive conversation data only. Apply offline; never replay service initialization.

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
-- END TARGET agent_conversation.sql

-- Remove inherited/default ACL leakage after creating the precise new objects.
REVOKE ALL ON ALL TABLES IN SCHEMA agent FROM PUBLIC,glowloom_app,glowloom_agent_runtime;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA agent FROM PUBLIC,glowloom_app,glowloom_agent_runtime;
REVOKE ALL ON SCHEMA agent_business FROM glowloom_agent_runtime;
REVOKE ALL ON ALL TABLES IN SCHEMA agent_business FROM glowloom_agent_runtime;
DO $gate$
DECLARE obj text;
BEGIN
 FOREACH obj IN ARRAY ARRAY['agent.runtime_sessions','agent.runtime_records','agent.runtime_document_versions','agent.runtime_events','agent.service_sessions','agent.service_runs','agent.service_control_inbox','agent.public_messages'] LOOP
  IF has_table_privilege('glowloom_app',obj,'SELECT,INSERT,UPDATE,DELETE')
  OR has_table_privilege('glowloom_agent_runtime',obj,'SELECT,INSERT,UPDATE,DELETE') THEN
   RAISE EXCEPTION 'Go/old runtime role still has runtime privileges on %; ownership or inherited role must be separately reviewed',obj;
  END IF;
 END LOOP;
 FOREACH obj IN ARRAY ARRAY['agent_business.run_authorization','agent_business.execution_grant','agent_business.control_decision','agent_business.quiesced_receipt','agent_business.proposal','agent_business.call_receipt','agent_business.source_authorization','agent_business.skill_version','identity.user','workspace.project','media.media_asset','billing.ledger_entry','operation.operation','infra.task_work','operation.development_trial'] LOOP
  IF has_table_privilege('glowloom_agent',obj,'SELECT,INSERT,UPDATE,DELETE') THEN
   RAISE EXCEPTION 'Agent role can access business table %; inherited/public permissions must be separately reviewed',obj;
  END IF;
 END LOOP;
 IF NOT has_table_privilege('glowloom_app','agent_business.source_authorization','SELECT')
 OR NOT has_table_privilege('glowloom_app','agent_business.source_authorization','INSERT')
 OR NOT has_table_privilege('glowloom_app','agent_business.source_authorization','UPDATE') THEN
  RAISE EXCEPTION 'Go business role lacks source authorization read/write permissions';
 END IF;
END
$gate$;
SELECT 'independent-agent-cutover-2026-11-04' AS cutover,
       current_database() AS database,current_setting('glowloom.cutover.proof')::jsonb AS evidence;
COMMIT;
