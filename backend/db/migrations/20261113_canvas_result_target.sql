-- Run once as schema owner on existing installations. New databases use schema.sql.
BEGIN;
SET LOCAL lock_timeout = '5s';
ALTER TABLE operation.prompt_submission ADD COLUMN target_content_version bigint;
-- Match the original saved revision, never the current contents of a node.
UPDATE operation.prompt_submission s
SET target_content_version = (n->>'content_version')::bigint
FROM canvas.collab_version v CROSS JOIN LATERAL jsonb_array_elements(v.document_json->'nodes') n
WHERE s.input_source->>'kind'='canvas_input'
  AND v.document_id=(s.input_source->>'canvas_id')::uuid
  AND v.revision=(s.input_source->>'canvas_revision')::bigint
  AND v.document_json->>'project_id'=s.project_id::text
  AND n->>'id'=s.input_source->>'node_id'
  AND n->>'input_revision'=s.input_source->>'input_revision';
ALTER TABLE operation.prompt_submission ADD CONSTRAINT ck_submission_result_target
  CHECK ((target_content_version IS NOT NULL) = (input_source->>'kind' IS NOT DISTINCT FROM 'canvas_input') AND target_content_version > 0);
UPDATE operation.operation o
SET source_context=jsonb_build_object('canvas_id',s.input_source->'canvas_id','node_id',s.input_source->'node_id','revision',s.input_source->'canvas_revision')
FROM operation.prompt_submission s
WHERE s.target_content_version IS NOT NULL AND o.id=s.operation_id AND o.project_id=s.project_id AND o.source_context IS NULL;
COMMIT;
