CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS workspaces (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 120),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS projects (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 160),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS projects_workspace_id_idx ON projects(workspace_id);

CREATE TABLE IF NOT EXISTS script_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 240),
    object_key text NOT NULL UNIQUE,
    content_hash text NOT NULL CHECK (content_hash ~ '^[a-f0-9]{64}$'),
    content_length bigint NOT NULL CHECK (content_length > 0),
    status text NOT NULL CHECK (status IN ('uploaded', 'analyzing', 'approved', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS script_revisions_project_id_idx ON script_revisions(project_id);

CREATE TABLE IF NOT EXISTS operations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    type text NOT NULL CHECK (type IN ('script_analysis')),
    status text NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
    progress smallint NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
    error_code text,
    error_message text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);

CREATE INDEX IF NOT EXISTS operations_project_id_idx ON operations(project_id);

CREATE TABLE IF NOT EXISTS outbox_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id uuid NOT NULL REFERENCES operations(id),
    topic text NOT NULL,
    event_key text NOT NULL,
    payload jsonb NOT NULL,
    attempts integer NOT NULL DEFAULT 0,
    published_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS outbox_unpublished_idx
    ON outbox_events(created_at)
    WHERE published_at IS NULL;

CREATE TABLE IF NOT EXISTS inbox_messages (
    message_id text PRIMARY KEY,
    topic text NOT NULL,
    consumed_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS script_analysis_drafts (
    script_revision_id uuid PRIMARY KEY REFERENCES script_revisions(id),
    source_hash text NOT NULL,
    analysis jsonb NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'approved')),
    created_at timestamptz NOT NULL DEFAULT now(),
    approved_at timestamptz
);

CREATE TABLE IF NOT EXISTS content_units (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    script_revision_id uuid NOT NULL REFERENCES script_revisions(id),
    episode_number integer NOT NULL CHECK (episode_number > 0),
    title text NOT NULL,
    start_offset integer NOT NULL CHECK (start_offset >= 0),
    end_offset integer NOT NULL CHECK (end_offset > start_offset),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (script_revision_id, episode_number)
);

CREATE TABLE IF NOT EXISTS narrative_units (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_unit_id uuid NOT NULL REFERENCES content_units(id),
    kind text NOT NULL CHECK (kind IN ('scene', 'dialogue', 'action')),
    text text NOT NULL,
    start_offset integer NOT NULL CHECK (start_offset >= 0),
    end_offset integer NOT NULL CHECK (end_offset > start_offset)
);

CREATE TABLE IF NOT EXISTS entities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    kind text NOT NULL CHECK (kind IN ('character', 'location', 'prop', 'costume')),
    canonical_name text NOT NULL,
    status text NOT NULL DEFAULT 'approved' CHECK (status IN ('approved', 'unresolved', 'rejected')),
    UNIQUE (project_id, kind, canonical_name)
);

CREATE TABLE IF NOT EXISTS entity_mentions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    narrative_unit_id uuid NOT NULL REFERENCES narrative_units(id),
    entity_id uuid REFERENCES entities(id),
    surface text NOT NULL,
    start_offset integer NOT NULL CHECK (start_offset >= 0),
    end_offset integer NOT NULL CHECK (end_offset > start_offset)
);

CREATE TABLE IF NOT EXISTS production_requirements (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    entity_id uuid NOT NULL REFERENCES entities(id),
    kind text NOT NULL,
    description text NOT NULL,
    status text NOT NULL DEFAULT 'unassessed' CHECK (status IN ('unassessed', 'required', 'not_required')),
    UNIQUE (project_id, entity_id, kind)
);
