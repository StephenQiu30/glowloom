-- Rollback must fail if tool inputs still exist; it must never discard user data.
ALTER TABLE canvas.node DROP CONSTRAINT node_node_action_check;
ALTER TABLE canvas.node ADD CONSTRAINT node_node_action_check
  CHECK (node_action IN ('resource','generate','edit'));
