-- Read-only old-deployment inventory. Run with psql -X -v ON_ERROR_STOP=1.
-- Contains IDs/status/cost only, never request bodies, goals or credentials.
\set ON_ERROR_STOP on
BEGIN ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT current_database() AS database, current_timestamp AS inventory_at,
       current_setting('transaction_read_only') AS read_only;

-- HEAD full schema has eight old tables. agent_checkpoint.sql may add the
-- optional ninth table graph_checkpoint; this query reports actual objects.
SELECT c.relname AS old_table, pg_get_userbyid(c.relowner) AS owner
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='agent' AND c.relkind IN ('r','p') ORDER BY c.relname;
SELECT count(*) AS actual_old_agent_table_count
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='agent' AND c.relkind IN ('r','p');
SELECT conrelid::regclass AS source, conname, confrelid::regclass AS target,
       pg_get_constraintdef(oid) AS definition
FROM pg_constraint WHERE contype='f' AND
 (connamespace='agent'::regnamespace OR confrelid IN
  (SELECT c.oid FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='agent'))
ORDER BY conrelid::regclass::text,conname;
SELECT rolname,rolcanlogin,rolsuper,rolcreatedb,rolcreaterole,rolinherit,rolbypassrls
FROM pg_roles WHERE rolname IN ('glowloom_app','glowloom_agent_runtime','glowloom_agent') ORDER BY rolname;
SELECT role.rolname AS granted_role,member.rolname AS member
FROM pg_auth_members m JOIN pg_roles role ON role.oid=m.roleid JOIN pg_roles member ON member.oid=m.member
WHERE role.rolname IN ('glowloom_app','glowloom_agent_runtime','glowloom_agent')
 OR member.rolname IN ('glowloom_app','glowloom_agent_runtime','glowloom_agent') ORDER BY 1,2;
SELECT n.nspname,c.relname,c.relacl FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='agent' ORDER BY c.relname;
SELECT n.nspname,n.nspacl FROM pg_namespace n WHERE n.nspname='agent';
SELECT n.nspname,c.relname,c.relacl,
 has_table_privilege('glowloom_agent_runtime',c.oid,'SELECT,INSERT,UPDATE,DELETE') AS old_runtime_access,
 has_table_privilege('glowloom_agent',c.oid,'SELECT,INSERT,UPDATE,DELETE') AS new_agent_access
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname NOT IN ('agent','pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%'
 AND c.relkind IN ('r','p','v','m','f') AND
 (has_table_privilege('glowloom_agent_runtime',c.oid,'SELECT,INSERT,UPDATE,DELETE')
 OR has_table_privilege('glowloom_agent',c.oid,'SELECT,INSERT,UPDATE,DELETE')) ORDER BY n.nspname,c.relname;
SELECT pg_get_userbyid(defaclrole) AS owner,defaclnamespace::regnamespace AS schema,defaclobjtype,defaclacl
FROM pg_default_acl WHERE defaclnamespace IN (0,'agent'::regnamespace);
SELECT d.deptype,pg_describe_object(d.classid,d.objid,d.objsubid) AS old_runtime_dependency
FROM pg_shdepend d WHERE d.dbid=(SELECT oid FROM pg_database WHERE datname=current_database())
 AND d.refclassid='pg_authid'::regclass AND d.refobjid=(SELECT oid FROM pg_roles WHERE rolname='glowloom_agent_runtime')
ORDER BY old_runtime_dependency;
SELECT conname,pg_get_constraintdef(oid) AS definition FROM pg_constraint
WHERE conrelid='operation.operation'::regclass AND conname IN
 ('ck_operation_agent_session','ck_operation_upload','operation_target_type_check','ck_operation_canvas_source') ORDER BY conname;

\ir agent_cutover_blockers.sql
SELECT * FROM jsonb_to_recordset(:'cutover_blocker_inventory'::jsonb)
 AS b(check_name text,blocked_rows bigint,reason text) ORDER BY check_name;

SELECT id,session_id,runtime_session_id,status,revision,control_revision FROM agent.harness_run ORDER BY id;
SELECT id,run_id,action,status,max_micros FROM agent.harness_admission ORDER BY run_id,id;
SELECT id,run_id,action,delivery_state,attempts FROM agent.harness_command ORDER BY run_id,id;
SELECT id,project_id,status,workflow_id,provider_request_key,reservation_id,settled_micros,cost_estimated,
       started_at,finished_at,target_type FROM operation.operation ORDER BY id;
SELECT id,status,workflow_id,unknown_count FROM operation.batch ORDER BY id;
SELECT id,operation_id,action,attempt,provider_key,provider_task_id,request_key,outcome,
       cost_micros,dispatch_started_at,(receipt IS NOT NULL) AS has_receipt
FROM operation.provider_call ORDER BY create_time,id;
SELECT id,operation_id,status,amount_micros,closed_at FROM billing.reservation ORDER BY id;
SELECT operation_id,entry_type,sum(amount_micros) AS amount_micros,count(*) AS entries
FROM billing.ledger_entry GROUP BY operation_id,entry_type ORDER BY operation_id,entry_type;
SELECT id,status,task_id,dispatch_started_at,tracking_ended_at,completed_at FROM catalog.provider_check ORDER BY id;
SELECT id,status,execution_id,dispatch_started_at,completed_at,(usage IS NOT NULL) AS has_usage
FROM subject.voice_job ORDER BY id;
SELECT pid,usename,application_name,state,xact_start FROM pg_stat_activity
WHERE datname=current_database() AND pid<>pg_backend_pid() AND backend_type='client backend' ORDER BY pid;

-- PostgreSQL cannot prove the state of Temporal schedules/runs (including
-- admin trials and agent.codex), remote providers, object writes or charges
-- not recorded here. Their independent clearance evidence is mandatory.
SELECT 'EXTERNAL_CLEARANCE_REQUIRED' AS gate,
 'Review all old Temporal namespaces/task queues/schedules and run histories; match every workflow/provider request, object side effect and final supplier charge to durable business receipts. No running/pending/unknown/continued execution or unresolved charge may remain.' AS requirement;
COMMIT;
