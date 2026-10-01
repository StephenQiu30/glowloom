-- Tool nodes retain editable input documents in the existing authoritative graph.
ALTER TABLE canvas.node DROP CONSTRAINT node_node_action_check;
ALTER TABLE canvas.node ADD CONSTRAINT node_node_action_check
  CHECK (node_action IN ('resource','generate','edit','tool'));
