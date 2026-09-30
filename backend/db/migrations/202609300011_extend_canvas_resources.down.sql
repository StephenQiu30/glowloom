ALTER TABLE canvas.node DROP CONSTRAINT ck_canvas_node_parent_self;
ALTER TABLE canvas.node DROP CONSTRAINT fk_canvas_node_parent;
ALTER TABLE canvas.node DROP CONSTRAINT ck_canvas_node_title;
ALTER TABLE canvas.node DROP COLUMN title;
