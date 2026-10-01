CREATE TABLE workspace.prompt_customization (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES workspace.organization(id),
  owner_id uuid NOT NULL REFERENCES identity."user"(id),
  operation text NOT NULL CHECK (btrim(operation) <> ''),
  mode text NOT NULL CHECK (mode IN ('inherit', 'append', 'rewrite')),
  content text NOT NULL CHECK (char_length(content) <= 12000),
  base_template_id uuid NOT NULL,
  revision integer NOT NULL DEFAULT 1 CHECK (revision > 0),
  create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT uq_prompt_customization_owner_operation UNIQUE(org_id, owner_id, operation),
  CONSTRAINT ck_prompt_customization_content CHECK (
    (mode = 'inherit' AND content = '') OR
    (mode IN ('append','rewrite') AND btrim(content) <> '')
  )
);
GRANT SELECT, INSERT, UPDATE ON workspace.prompt_customization TO lanverse_app;
