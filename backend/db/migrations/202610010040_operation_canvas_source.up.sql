ALTER TABLE operation.operation ADD COLUMN source_context jsonb;
ALTER TABLE operation.operation ADD CONSTRAINT ck_operation_canvas_source CHECK (
  source_context IS NULL OR (
    target_type='free' AND origin='canvas' AND jsonb_typeof(source_context)='object'
    AND source_context ?& ARRAY['canvas_id','node_id','revision']
    AND (source_context - ARRAY['canvas_id','node_id','row_id','revision'])='{}'::jsonb
    AND jsonb_typeof(source_context->'canvas_id')='string'
    AND jsonb_typeof(source_context->'node_id')='string'
    AND jsonb_typeof(source_context->'revision')='number'
    AND (source_context->>'revision')::bigint > 0
    AND (NOT source_context ? 'row_id' OR jsonb_typeof(source_context->'row_id')='string')
  )
);
CREATE INDEX ix_operation_canvas_source ON operation.operation
  ((source_context->>'canvas_id'),(source_context->>'node_id'),create_time DESC)
  WHERE source_context IS NOT NULL AND NOT is_delete;
