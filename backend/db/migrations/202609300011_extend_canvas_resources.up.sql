-- Preserve the published note schema and existing documents while extending presentation data.
ALTER TABLE canvas.node ADD COLUMN title text NOT NULL DEFAULT '备注';
ALTER TABLE canvas.node ADD CONSTRAINT ck_canvas_node_title CHECK (char_length(title) BETWEEN 1 AND 128);
ALTER TABLE canvas.node ADD CONSTRAINT fk_canvas_node_parent FOREIGN KEY(document_id,parent_id)
  REFERENCES canvas.node(document_id,id) DEFERRABLE INITIALLY DEFERRED NOT VALID;
ALTER TABLE canvas.node ADD CONSTRAINT ck_canvas_node_parent_self CHECK(parent_id IS NULL OR parent_id<>id) NOT VALID;
