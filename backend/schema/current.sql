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

CREATE TABLE IF NOT EXISTS operations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    type text NOT NULL CHECK (type IN ('script_analysis', 'fixture_candidate', 'generation', 'delivery', 'quality')),
    status text NOT NULL CHECK (status IN ('queued', 'running', 'waiting_user', 'partial', 'blocked', 'unknown', 'succeeded', 'failed', 'cancelled')),
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

-- The following tables are the current platform schema for modules M01-M15.
-- Each module owns its tables; schema-init applies this file only to an empty
-- business database, so this contract contains no migration or compatibility
-- statements.

CREATE TABLE IF NOT EXISTS iam_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    identity_subject text NOT NULL UNIQUE,
    email text,
    password_hash text,
    display_name text NOT NULL CHECK (length(trim(display_name)) BETWEEN 1 AND 160),
    status text NOT NULL CHECK (status IN ('active', 'suspended', 'removed')),
    email_verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS iam_roles (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code text NOT NULL UNIQUE CHECK (code IN ('admin', 'user', 'ban')),
    scope text NOT NULL CHECK (scope IN ('workspace')),
    is_system boolean NOT NULL DEFAULT false
);

CREATE TABLE IF NOT EXISTS iam_memberships (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    user_id uuid NOT NULL REFERENCES iam_users(id),
    role_id uuid NOT NULL REFERENCES iam_roles(id),
    status text NOT NULL CHECK (status IN ('invited', 'active', 'suspended', 'removed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, user_id)
);

CREATE TABLE IF NOT EXISTS iam_project_grants (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    membership_id uuid NOT NULL REFERENCES iam_memberships(id),
    role_id uuid NOT NULL REFERENCES iam_roles(id),
    status text NOT NULL CHECK (status IN ('active', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, membership_id)
);

CREATE TABLE IF NOT EXISTS iam_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    family_id uuid NOT NULL DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES iam_users(id),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz
);

CREATE UNIQUE INDEX IF NOT EXISTS iam_users_email_unique_idx ON iam_users(email) WHERE email IS NOT NULL;
CREATE INDEX IF NOT EXISTS iam_sessions_family_idx ON iam_sessions(family_id, revoked_at, expires_at);

INSERT INTO iam_roles (code, scope, is_system) VALUES
    ('admin', 'workspace', true),
    ('user', 'workspace', true),
    ('ban', 'workspace', true);

CREATE TABLE IF NOT EXISTS iam_service_identities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    name text NOT NULL,
    scopes jsonb NOT NULL DEFAULT '[]',
    expires_at timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'rotating', 'suspended', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    actor_type text NOT NULL CHECK (actor_type IN ('user', 'service', 'system')),
    actor_id text NOT NULL,
    action text NOT NULL,
    object_type text NOT NULL,
    object_id uuid NOT NULL,
    before_state jsonb NOT NULL,
    after_state jsonb NOT NULL,
    before_hash text NOT NULL CHECK (before_hash ~ '^[0-9a-f]{64}$'),
    after_hash text NOT NULL CHECK (after_hash ~ '^[0-9a-f]{64}$'),
    request_id text NOT NULL CHECK (length(trim(request_id)) BETWEEN 1 AND 200),
    reason text NOT NULL CHECK (length(trim(reason)) BETWEEN 1 AND 500),
    result text NOT NULL CHECK (result IN ('succeeded', 'denied', 'failed')),
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS prj_brief_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    revision_no integer NOT NULL,
    target_platforms jsonb NOT NULL DEFAULT '[]',
    target_duration_ticks bigint,
    aspect_ratio text,
    width integer,
    height integer,
    frame_rate_num integer,
    frame_rate_den integer,
    audio_sample_rate integer,
    language text,
    usage_cap jsonb,
    status text NOT NULL CHECK (status IN ('draft', 'approved', 'superseded')),
    content_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, revision_no)
);

CREATE TABLE IF NOT EXISTS prj_content_units (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    kind text NOT NULL,
    title text NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'active', 'paused', 'completed', 'archived')),
    ordinal integer,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS prj_content_order_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    revision_no integer NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'approved', 'superseded')),
    content_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, revision_no)
);

CREATE TABLE IF NOT EXISTS prj_content_order_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_revision_id uuid NOT NULL REFERENCES prj_content_order_revisions(id),
    content_unit_id uuid NOT NULL REFERENCES prj_content_units(id),
    ordinal integer NOT NULL CHECK (ordinal > 0),
    UNIQUE (order_revision_id, ordinal),
    UNIQUE (order_revision_id, content_unit_id)
);

CREATE TABLE IF NOT EXISTS media_artifacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    content_hash text NOT NULL CHECK (content_hash ~ '^[a-f0-9]{64}$'),
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    media_type text NOT NULL,
    purpose text NOT NULL,
    rights_scope_hash text,
    retention_class text NOT NULL DEFAULT 'standard',
    status text NOT NULL CHECK (status IN ('staging', 'ready', 'quarantined', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (id, project_id)
);

CREATE INDEX IF NOT EXISTS media_artifacts_project_idx ON media_artifacts(project_id, created_at DESC);

CREATE TABLE IF NOT EXISTS media_artifact_locations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    artifact_id uuid NOT NULL,
    storage_profile text NOT NULL,
    bucket text NOT NULL,
    object_key text NOT NULL,
    object_version_id text NOT NULL,
    size_bytes bigint NOT NULL CHECK (size_bytes > 0),
    content_hash text NOT NULL CHECK (content_hash ~ '^[a-f0-9]{64}$'),
    etag text,
    status text NOT NULL CHECK (status IN ('staged', 'verified', 'active', 'retiring', 'retired', 'quarantined', 'missing', 'deletion_pending', 'deleted')),
    verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (storage_profile, bucket, object_key, object_version_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS media_artifact_locations_active_idx ON media_artifact_locations(artifact_id) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS nar_source_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    content_unit_id uuid REFERENCES prj_content_units(id),
    artifact_id uuid NOT NULL REFERENCES media_artifacts(id),
    name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 240),
    source_type text NOT NULL CHECK (source_type IN ('txt', 'markdown', 'docx')),
    status text NOT NULL CHECK (status IN ('uploaded', 'analyzing', 'waiting_user', 'approved', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (artifact_id, project_id) REFERENCES media_artifacts(id, project_id)
);

CREATE INDEX IF NOT EXISTS nar_source_revisions_project_idx ON nar_source_revisions(project_id, created_at DESC);

CREATE TABLE IF NOT EXISTS nar_analysis_drafts (
    source_revision_id uuid PRIMARY KEY REFERENCES nar_source_revisions(id),
    source_hash text NOT NULL,
    analysis jsonb NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'approved')),
    created_at timestamptz NOT NULL DEFAULT now(),
    approved_at timestamptz
);

CREATE TABLE IF NOT EXISTS nar_import_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    operation_id uuid NOT NULL REFERENCES operations(id),
    source_manifest_hash text NOT NULL,
    parser_config_hash text NOT NULL,
    status text NOT NULL CHECK (status IN ('queued', 'parsing', 'partial', 'completed', 'failed', 'cancelled', 'superseded')),
    parse_report jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS nar_analysis_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    root_operation_id uuid NOT NULL REFERENCES operations(id),
    source_manifest_hash text NOT NULL,
    current_stage text NOT NULL CHECK (current_stage IN ('breakdown', 'narrative', 'knowledge')),
    current_stage_generation integer NOT NULL DEFAULT 1,
    current_gate text NOT NULL,
    status text NOT NULL CHECK (status IN ('queued', 'analyzing', 'waiting_user', 'materializing', 'partial', 'failed', 'cancelled', 'completed')),
    input_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS nar_episode_breakdown_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    analysis_run_id uuid NOT NULL REFERENCES nar_analysis_runs(id),
    revision_no integer NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'approved', 'superseded')),
    segmentation_hash text NOT NULL,
    coverage_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (analysis_run_id, revision_no)
);

CREATE TABLE IF NOT EXISTS nar_episode_candidates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    breakdown_revision_id uuid NOT NULL REFERENCES nar_episode_breakdown_revisions(id),
    temporary_key text NOT NULL,
    ordinal integer NOT NULL,
    title text NOT NULL,
    rule_code text NOT NULL,
    confidence numeric(5,4),
    decision text NOT NULL CHECK (decision IN ('pending', 'accepted', 'ignored', 'rejected')),
    UNIQUE (breakdown_revision_id, temporary_key),
    UNIQUE (breakdown_revision_id, ordinal)
);

CREATE TABLE IF NOT EXISTS nar_narrative_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    breakdown_revision_id uuid REFERENCES nar_episode_breakdown_revisions(id),
    revision_no integer NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'proposed', 'approved', 'current', 'superseded')),
    content_hash text NOT NULL,
    completeness text NOT NULL CHECK (completeness IN ('complete', 'partial', 'incomplete', 'stale')),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, revision_no)
);

CREATE TABLE IF NOT EXISTS nar_scenes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    narrative_revision_id uuid NOT NULL REFERENCES nar_narrative_revisions(id),
    content_unit_id uuid NOT NULL REFERENCES prj_content_units(id),
    ordinal integer NOT NULL,
    heading text NOT NULL,
    location_hint text,
    start_offset integer NOT NULL,
    end_offset integer NOT NULL,
    UNIQUE (narrative_revision_id, content_unit_id, ordinal)
);

CREATE TABLE IF NOT EXISTS nar_beats (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    scene_id uuid NOT NULL REFERENCES nar_scenes(id),
    ordinal integer NOT NULL,
    goal text,
    conflict text,
    UNIQUE (scene_id, ordinal)
);

CREATE TABLE IF NOT EXISTS nar_production_element_mentions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    narrative_revision_id uuid NOT NULL REFERENCES nar_narrative_revisions(id),
    scene_id uuid REFERENCES nar_scenes(id),
    beat_id uuid REFERENCES nar_beats(id),
    element_type text NOT NULL CHECK (element_type IN ('character', 'location', 'prop', 'costume')),
    surface_text text NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'rejected')),
    start_offset integer NOT NULL,
    end_offset integer NOT NULL
);

CREATE TABLE IF NOT EXISTS pk_entities (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    type text NOT NULL CHECK (type IN ('character', 'location', 'prop', 'costume')),
    status text NOT NULL CHECK (status IN ('active', 'archived')),
    canonical_name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, type, canonical_name)
);

CREATE TABLE IF NOT EXISTS pk_mention_resolutions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    mention_id uuid NOT NULL REFERENCES nar_production_element_mentions(id),
    narrative_revision_id uuid NOT NULL REFERENCES nar_narrative_revisions(id),
    action text NOT NULL CHECK (action IN ('link', 'create', 'defer', 'reject')),
    entity_id uuid REFERENCES pk_entities(id),
    reason text,
    status text NOT NULL CHECK (status IN ('current', 'superseded')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS pk_production_requirement_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    stable_key text NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'active', 'archived')),
    UNIQUE (project_id, stable_key)
);

CREATE TABLE IF NOT EXISTS pk_production_requirement_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id uuid NOT NULL REFERENCES pk_production_requirement_items(id),
    revision_no integer NOT NULL,
    type text NOT NULL,
    purpose text NOT NULL,
    variant text,
    quantity numeric,
    unit text,
    decision text NOT NULL CHECK (decision IN ('required', 'not_required', 'deferred', 'rejected')),
    status text NOT NULL CHECK (status IN ('current', 'superseded')),
    UNIQUE (item_id, revision_no)
);

CREATE TABLE IF NOT EXISTS sht_shots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    content_unit_id uuid NOT NULL REFERENCES prj_content_units(id),
    shot_key text NOT NULL,
    ordinal integer NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'approved', 'locked', 'archived')),
    source_beat_id uuid REFERENCES nar_beats(id),
    UNIQUE (content_unit_id, shot_key),
    UNIQUE (content_unit_id, ordinal)
);

CREATE TABLE IF NOT EXISTS sht_shot_plan_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    content_unit_id uuid NOT NULL REFERENCES prj_content_units(id),
    revision_no integer NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'approved', 'superseded')),
    content_hash text NOT NULL,
    UNIQUE (content_unit_id, revision_no)
);

CREATE TABLE IF NOT EXISTS m06_agent_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    operation_id uuid NOT NULL REFERENCES operations(id),
    skill_id text NOT NULL,
    stage text NOT NULL,
    stage_generation integer NOT NULL DEFAULT 1,
    request_hash text NOT NULL,
    status text NOT NULL CHECK (status IN ('accepted', 'running', 'partial', 'failed', 'cancelled', 'expired', 'succeeded')),
    input_snapshot_hash text NOT NULL,
    result_hash text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, skill_id, stage, stage_generation, request_hash)
);

CREATE TABLE IF NOT EXISTS m06_proposal_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_run_id uuid NOT NULL REFERENCES m06_agent_runs(id),
    target_module text NOT NULL,
    target_command text NOT NULL,
    payload jsonb NOT NULL,
    decision text NOT NULL CHECK (decision IN ('pending', 'accepted', 'edited', 'rejected', 'deferred', 'stale')),
    read_set_hash text NOT NULL,
    write_set_hash text NOT NULL
);

CREATE TABLE IF NOT EXISTS gen_plans (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    target_type text NOT NULL,
    target_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'preflight_ready', 'blocked', 'awaiting_approval', 'approved', 'rejected', 'superseded', 'cancelled')),
    execution_disposition text CHECK (execution_disposition IN ('start_now', 'hold')),
    input_snapshot_hash text NOT NULL,
    prompt_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS gen_plan_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id uuid NOT NULL REFERENCES gen_plans(id),
    ordinal integer NOT NULL,
    capability_key text NOT NULL,
    prompt text NOT NULL,
    status text NOT NULL CHECK (status IN ('proposed', 'selected', 'excluded', 'completed', 'failed')),
    UNIQUE (plan_id, ordinal)
);

CREATE TABLE IF NOT EXISTS exec_generation_jobs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_item_id uuid NOT NULL REFERENCES gen_plan_items(id),
    operation_id uuid NOT NULL REFERENCES operations(id),
    status text NOT NULL CHECK (status IN ('queued', 'running', 'waiting_external', 'unknown', 'succeeded', 'failed', 'cancelled')),
    current_attempt_id uuid,
    UNIQUE (plan_item_id)
);

CREATE TABLE IF NOT EXISTS exec_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id uuid NOT NULL REFERENCES exec_generation_jobs(id),
    attempt_no integer NOT NULL,
    status text NOT NULL CHECK (status IN ('created', 'submitted', 'running', 'succeeded', 'failed', 'unknown', 'cancelled')),
    external_job_id text,
    result_certainty text CHECK (result_certainty IN ('created', 'not_created', 'unknown')),
    UNIQUE (job_id, attempt_no)
);

CREATE TABLE IF NOT EXISTS media_candidates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    job_id uuid REFERENCES exec_generation_jobs(id),
    target_type text NOT NULL,
    target_id uuid NOT NULL,
    artifact_id uuid NOT NULL,
    status text NOT NULL CHECK (status IN ('ready', 'quarantined', 'retired')),
    created_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (artifact_id, project_id) REFERENCES media_artifacts(id, project_id)
);

CREATE TABLE IF NOT EXISTS media_selection_decisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    target_type text NOT NULL,
    target_id uuid NOT NULL,
    selection_purpose text NOT NULL,
    candidate_id uuid NOT NULL REFERENCES media_candidates(id),
    status text NOT NULL CHECK (status IN ('current', 'superseded')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS qa_evaluations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    subject_hash text NOT NULL,
    evaluator_version text NOT NULL,
    status text NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'stale')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS qa_issues (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    evaluation_id uuid NOT NULL REFERENCES qa_evaluations(id),
    rule_code text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('info', 'warning', 'error', 'blocker')),
    status text NOT NULL CHECK (status IN ('open', 'acknowledged', 'rejected', 'resolved', 'accepted_risk')),
    evidence jsonb NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS usage_reservations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    plan_id uuid REFERENCES gen_plans(id),
    quantity numeric NOT NULL CHECK (quantity >= 0),
    unit text NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'released', 'settled', 'cancelled'))
);

CREATE TABLE IF NOT EXISTS usage_entries (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    quantity numeric NOT NULL,
    unit text NOT NULL,
    kind text NOT NULL,
    provider_record_ref text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS review_packages (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    content_hash text NOT NULL,
    manifest jsonb NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'active', 'closed', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS review_decisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id uuid NOT NULL REFERENCES review_packages(id),
    decision text NOT NULL CHECK (decision IN ('approved', 'changes_requested', 'rejected', 'withdrawn')),
    actor_id text NOT NULL,
    reason text,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS delivery_assembly_revisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    content_unit_id uuid NOT NULL REFERENCES prj_content_units(id),
    revision_no integer NOT NULL,
    timeline_timebase_num integer NOT NULL,
    timeline_timebase_den integer NOT NULL,
    track_manifest jsonb NOT NULL,
    content_hash text NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'approved', 'superseded')),
    UNIQUE (content_unit_id, revision_no)
);

CREATE TABLE IF NOT EXISTS delivery_builds (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    assembly_revision_id uuid NOT NULL REFERENCES delivery_assembly_revisions(id),
    operation_id uuid NOT NULL REFERENCES operations(id),
    input_hash text NOT NULL,
    status text NOT NULL CHECK (status IN ('preflight', 'queued', 'running', 'failed', 'cancelled', 'succeeded')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS delivery_snapshots (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    build_id uuid NOT NULL REFERENCES delivery_builds(id) UNIQUE,
    input_manifest_hash text NOT NULL,
    output_manifest_hash text NOT NULL,
    manifest jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS gov_rights_declarations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    owner text NOT NULL,
    source text NOT NULL,
    uses jsonb NOT NULL,
    regions jsonb NOT NULL,
    expires_at timestamptz,
    status text NOT NULL CHECK (status IN ('active', 'revoked', 'expired'))
);

CREATE TABLE IF NOT EXISTS gov_policy_evaluations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES projects(id),
    subject_hash text NOT NULL,
    result text NOT NULL CHECK (result IN ('allowed', 'blocked', 'unknown')),
    reasons jsonb NOT NULL DEFAULT '[]',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS tpl_templates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    kind text NOT NULL,
    status text NOT NULL CHECK (status IN ('draft', 'published', 'archived')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS int_api_clients (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    name text NOT NULL,
    scopes jsonb NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'revoked')),
    expires_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS int_webhook_subscriptions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    endpoint_ref text NOT NULL,
    events jsonb NOT NULL,
    status text NOT NULL CHECK (status IN ('active', 'paused', 'revoked')),
    secret_version integer NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS ops_idempotency_records (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id uuid REFERENCES workspaces(id),
    actor_id text NOT NULL,
    method text NOT NULL,
    route_template text NOT NULL,
    idempotency_key text NOT NULL,
    request_hash text NOT NULL,
    response_status smallint,
    result_summary jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, actor_id, method, route_template, idempotency_key)
);

CREATE TABLE IF NOT EXISTS ops_operation_steps (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    operation_id uuid NOT NULL REFERENCES operations(id),
    step_code text NOT NULL,
    status text NOT NULL,
    summary jsonb NOT NULL DEFAULT '{}',
    started_at timestamptz,
    ended_at timestamptz,
    UNIQUE (operation_id, step_code)
);

CREATE TABLE IF NOT EXISTS ops_task_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id uuid NOT NULL,
    attempt_no integer NOT NULL,
    worker_kind text NOT NULL,
    status text NOT NULL,
    error_code text,
    started_at timestamptz NOT NULL DEFAULT now(),
    ended_at timestamptz,
    UNIQUE (task_id, attempt_no)
);

CREATE TABLE IF NOT EXISTS ops_search_projection_checkpoints (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    projection_name text NOT NULL,
    workspace_id uuid NOT NULL REFERENCES workspaces(id),
    project_id uuid NOT NULL REFERENCES projects(id),
    index_schema_hash text NOT NULL,
    indexed_revision_set_hash text NOT NULL,
    status text NOT NULL CHECK (status IN ('current', 'stale', 'rebuilding', 'unavailable')),
    as_of timestamptz NOT NULL DEFAULT now(),
    UNIQUE (projection_name, workspace_id, project_id)
);

CREATE INDEX IF NOT EXISTS ix_iam_memberships_workspace ON iam_memberships(workspace_id, status);
CREATE INDEX IF NOT EXISTS ix_audit_events_workspace_time ON audit_events(workspace_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS ix_nar_analysis_runs_project ON nar_analysis_runs(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_nar_scenes_revision ON nar_scenes(narrative_revision_id, content_unit_id, ordinal);
CREATE INDEX IF NOT EXISTS ix_pk_entities_project ON pk_entities(project_id, type, canonical_name);
CREATE INDEX IF NOT EXISTS ix_sht_shots_content_unit ON sht_shots(content_unit_id, ordinal);
CREATE INDEX IF NOT EXISTS ix_m06_agent_runs_project ON m06_agent_runs(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_gen_plans_project ON gen_plans(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_exec_jobs_operation ON exec_generation_jobs(operation_id);
CREATE INDEX IF NOT EXISTS ix_media_candidates_target ON media_candidates(project_id, target_type, target_id);
CREATE INDEX IF NOT EXISTS ix_qa_issues_status ON qa_issues(status, severity);
CREATE INDEX IF NOT EXISTS ix_usage_entries_project_time ON usage_entries(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_review_packages_project ON review_packages(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ix_delivery_builds_project ON delivery_builds(project_id, created_at DESC);

-- Defense-in-depth tenant policies. Runtime business connections set
-- `app.workspace_id` inside each transaction. Tables whose tenant is reached
-- through project_id use the same boundary in their module query predicates;
-- only tables with a direct workspace_id use this generic policy.
ALTER TABLE projects ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON projects;
CREATE POLICY tenant_isolation ON projects
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE iam_memberships ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON iam_memberships;
CREATE POLICY tenant_isolation ON iam_memberships
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE iam_project_grants ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON iam_project_grants;
CREATE POLICY tenant_isolation ON iam_project_grants
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE iam_sessions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON iam_sessions;
CREATE POLICY tenant_isolation ON iam_sessions
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE iam_service_identities ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON iam_service_identities;
CREATE POLICY tenant_isolation ON iam_service_identities
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON audit_events;
CREATE POLICY tenant_isolation ON audit_events
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE media_artifacts ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON media_artifacts;
CREATE POLICY tenant_isolation ON media_artifacts
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE tpl_templates ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON tpl_templates;
CREATE POLICY tenant_isolation ON tpl_templates
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE int_api_clients ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON int_api_clients;
CREATE POLICY tenant_isolation ON int_api_clients
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE int_webhook_subscriptions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON int_webhook_subscriptions;
CREATE POLICY tenant_isolation ON int_webhook_subscriptions
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE ops_idempotency_records ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON ops_idempotency_records;
CREATE POLICY tenant_isolation ON ops_idempotency_records
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);

ALTER TABLE ops_search_projection_checkpoints ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation ON ops_search_projection_checkpoints;
CREATE POLICY tenant_isolation ON ops_search_projection_checkpoints
    USING (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid)
    WITH CHECK (workspace_id = NULLIF(current_setting('app.workspace_id', true), '')::uuid);
