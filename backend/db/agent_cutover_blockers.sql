-- Read-only shared predicate source for inventory and the locked cutover gate.
-- Missing tables/columns are schema drift, not evidence that work is settled.
WITH blockers AS (
SELECT 'agent.session.turns' AS check_name, count(*) AS blocked_rows,
       'Nonterminal old bounded-Agent turns' AS reason
FROM agent.session s CROSS JOIN LATERAL jsonb_array_elements(s.turns) turn
WHERE coalesce(turn->>'status','') NOT IN ('completed','failed','stopped','expired')
UNION ALL
SELECT 'agent.harness_run', count(*), 'Nonterminal old Harness runs'
FROM agent.harness_run WHERE status NOT IN ('stopped','rejected','failed','completed','partially_completed')
UNION ALL
SELECT 'agent.harness_command', count(*), 'Unacknowledged old control delivery'
FROM agent.harness_command WHERE delivery_state NOT IN ('acknowledged','superseded')
UNION ALL
SELECT 'agent.harness_admission', count(*), 'Unsettled or unknown old action admissions'
FROM agent.harness_admission WHERE status <> 'settled'
UNION ALL
SELECT 'operation.operation.active', count(*), 'Nonterminal operations; no old execution is adopted by the new worker'
FROM operation.operation WHERE status NOT IN ('completed','failed','cancelled','expired')
UNION ALL
SELECT 'operation.operation.agent_session', count(*), 'Old target needs separately reviewed current resource ownership; never rewrite or delete business/ledger rows here'
FROM operation.operation WHERE target_type='agent_session'
UNION ALL
SELECT 'operation.operation.settlement', count(*), 'Missing final settlement or explicitly estimated cost still needs reconciliation'
FROM operation.operation WHERE (started_at IS NOT NULL AND settled_micros IS NULL) OR cost_estimated
UNION ALL
SELECT 'operation.batch', count(*), 'Nonterminal batches or unresolved member outcomes'
FROM operation.batch WHERE status NOT IN ('finished','cancelled','expired') OR unknown_count<>0
UNION ALL
SELECT 'operation.provider_call', count(*), 'Unknown provider outcomes or missing cost; query/read-like calls are not assumed free'
FROM operation.provider_call WHERE outcome IN ('timeout','unknown') OR cost_micros IS NULL
UNION ALL
SELECT 'billing.reservation', count(*), 'Held reservations or missing closure receipts'
FROM billing.reservation WHERE status='held' OR closed_at IS NULL
UNION ALL
SELECT 'catalog.model_validation', count(*), 'Unfinished/unknown model validation'
FROM catalog.model_validation WHERE status NOT IN ('succeeded','failed') OR completed_at IS NULL
UNION ALL
SELECT 'catalog.provider_check', count(*), 'Unfinished provider checks or external tasks still tracked'
FROM catalog.provider_check WHERE status NOT IN ('completed','failed') OR completed_at IS NULL
 OR (task_id<>'' AND tracking_ended_at IS NULL)
UNION ALL
SELECT 'media.package_job', count(*), 'Package reconciliation is unfinished'
FROM media.package_job WHERE status NOT IN ('succeeded','cancelled')
UNION ALL
SELECT 'media.purge_job', count(*), 'Unfinished or unconfirmed purge execution'
FROM media.purge_job WHERE status NOT IN ('succeeded','partial_failed','failed','cancelled')
 OR needs_reconciliation OR execution_unconfirmed OR NOT process_ended
UNION ALL
SELECT 'media.transfer_job', count(*), 'Unfinished or unconfirmed transfer execution'
FROM media.transfer_job WHERE status NOT IN ('succeeded','partial_failed','failed','cancelled')
 OR needs_reconciliation OR execution_unconfirmed OR NOT process_ended
UNION ALL
SELECT 'media.storage_work_intent', count(*), 'Open object-storage admission; external writes must be reconciled'
FROM media.storage_work_intent WHERE state='open' OR closed_at IS NULL
UNION ALL
SELECT 'media.media_asset', count(*), 'Old upload/processing work is unfinished'
FROM media.media_asset WHERE status IN ('uploading','processing')
UNION ALL
SELECT 'mediatool.depth_job', count(*), 'Depth process/output is unfinished or unknown'
FROM mediatool.depth_job WHERE status NOT IN ('succeeded','failed','cancelled')
 OR needs_reconciliation OR execution_unconfirmed OR active_worker IS NOT NULL OR process_state NOT IN ('none','ended')
UNION ALL
SELECT 'mediatool.export_job', count(*), 'Export work/output is unfinished'
FROM mediatool.export_job WHERE status NOT IN ('succeeded','failed','cancelled') OR active_worker IS NOT NULL
UNION ALL
SELECT 'mediatool.transcription_job', count(*), 'Transcription inference/output is unfinished or unknown'
FROM mediatool.transcription_job WHERE status NOT IN ('succeeded','failed','cancelled')
 OR active_worker IS NOT NULL OR inference_state NOT IN ('none','terminal')
UNION ALL
SELECT 'script.import_state', count(*), 'Import IO or reconciliation is unfinished'
FROM script.import_state WHERE status NOT IN ('succeeded','failed','cancelled')
 OR needs_reconciliation OR io_state NOT IN ('idle','ended')
UNION ALL
SELECT 'script.command_state', count(*), 'Script command IO is unfinished or unknown'
FROM script.command_state WHERE status NOT IN ('completed','cancelled') OR io_state NOT IN ('idle','ended')
UNION ALL
SELECT 'workspace.project_copy_job', count(*), 'Project copy or reconciliation is unfinished'
FROM workspace.project_copy_job WHERE status NOT IN ('succeeded','failed','cancelled') OR needs_reconciliation OR execution_unconfirmed
UNION ALL
SELECT 'workspace.project_retention_job', count(*), 'Old retention work remains pending'
FROM workspace.project_retention_job WHERE status NOT IN ('completed','skipped') OR completed_at IS NULL
UNION ALL
SELECT 'subject.voice_job', count(*), 'Voice dispatch/ingest or supplier cost evidence is unfinished'
FROM subject.voice_job WHERE status NOT IN ('succeeded','failed') OR completed_at IS NULL
 OR (dispatch_started_at IS NOT NULL AND (usage IS NULL OR upstream_cost='{}'::jsonb))
UNION ALL
SELECT 'identity.directory_export', count(*), 'Directory export work is unfinished'
FROM identity.directory_export WHERE status IN ('pending','running')
UNION ALL
SELECT 'infra.outbox', count(*), 'Undelivered old events can start work after cutover'
FROM infra.outbox WHERE published_at IS NULL
 )
SELECT jsonb_agg(to_jsonb(blockers) ORDER BY check_name) AS cutover_blocker_inventory FROM blockers
\gset
