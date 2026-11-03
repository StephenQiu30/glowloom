-- Additive only: historical missing evidence remains unknown. No ledger rewrite.
ALTER TABLE catalog.price_rule_version ADD COLUMN IF NOT EXISTS source_url text;
ALTER TABLE catalog.price_rule_version ADD COLUMN IF NOT EXISTS reviewed_at timestamptz;
CREATE INDEX IF NOT EXISTS ix_operation_canvas_billing_target ON operation.operation
 (project_id, (source_context->>'canvas_id'), (source_context->>'node_id'), (COALESCE(source_context->>'row_id','')))
 WHERE NOT is_delete AND source_context IS NOT NULL;
