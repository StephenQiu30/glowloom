DROP INDEX operation.ix_operation_canvas_source;
ALTER TABLE operation.operation DROP CONSTRAINT ck_operation_canvas_source;
ALTER TABLE operation.operation DROP COLUMN source_context;
