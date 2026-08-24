-- Lanverse compatibility runtime baseline.
-- This immutable snapshot is executed transactionally by lanverse-migrate.
-- Do not edit after release; add a new numbered migration instead.

CREATE TABLE idn_user_accounts (
	id UUID NOT NULL,
	email_normalized VARCHAR(320) NOT NULL,
	password_hash TEXT NOT NULL,
	token_version INTEGER NOT NULL,
	display_name VARCHAR(80) NOT NULL,
	avatar_url TEXT,
	status VARCHAR(20) NOT NULL,
	last_login_at TIMESTAMP WITH TIME ZONE,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_idn_user_status CHECK (status IN ('active', 'deactivated')),
	CONSTRAINT ck_idn_user_token_version CHECK (token_version >= 1),
	UNIQUE (email_normalized)
);

CREATE INDEX ix_idn_user_accounts_status ON idn_user_accounts (status);

CREATE TABLE idn_workspaces (
	id UUID NOT NULL,
	name VARCHAR(120) NOT NULL,
	status VARCHAR(20) NOT NULL,
	revision INTEGER NOT NULL,
	archived_at TIMESTAMP WITH TIME ZONE,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_idn_workspace_status CHECK (status IN ('active', 'archived')),
	CONSTRAINT ck_idn_workspace_revision CHECK (revision >= 1)
);

CREATE INDEX ix_idn_workspaces_status ON idn_workspaces (status);

CREATE TABLE prod_model_capabilities (
	id UUID NOT NULL,
	provider VARCHAR(60) NOT NULL,
	model VARCHAR(160) NOT NULL,
	kind VARCHAR(20) NOT NULL,
	config_version INTEGER NOT NULL,
	input_types JSONB NOT NULL,
	parameter_schema JSONB NOT NULL,
	limits JSONB NOT NULL,
	pricing JSONB,
	status VARCHAR(20) NOT NULL,
	unavailable_reason VARCHAR(100),
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_prod_capability_kind CHECK (kind IN ('text', 'image', 'video')),
	CONSTRAINT ck_prod_capability_status CHECK (status IN ('active', 'inactive', 'unavailable')),
	CONSTRAINT ck_prod_capability_config_version CHECK (config_version >= 1),
	CONSTRAINT uq_prod_capability_configuration UNIQUE (provider, model, kind, config_version),
	CONSTRAINT uq_prod_capability_id_version UNIQUE (id, config_version)
);

CREATE INDEX ix_prod_capability_kind_status ON prod_model_capabilities (kind, status);

CREATE TABLE gov_audit_events (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	actor_id UUID NOT NULL,
	action VARCHAR(80) NOT NULL,
	target_type VARCHAR(60) NOT NULL,
	target_id UUID NOT NULL,
	result VARCHAR(20) NOT NULL,
	trace_id VARCHAR(64) NOT NULL,
	metadata JSONB NOT NULL,
	occurred_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_gov_audit_result CHECK (result IN ('succeeded', 'denied', 'failed')),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id),
	FOREIGN KEY(actor_id) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_gov_audit_workspace_target_occurred ON gov_audit_events (workspace_id, target_type, target_id, occurred_at);

CREATE INDEX ix_gov_audit_workspace_occurred ON gov_audit_events (workspace_id, occurred_at);

CREATE INDEX ix_gov_audit_workspace_action_occurred ON gov_audit_events (workspace_id, action, occurred_at);

CREATE INDEX ix_gov_audit_workspace_actor_occurred ON gov_audit_events (workspace_id, actor_id, occurred_at);

CREATE TABLE idn_memberships (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	user_id UUID NOT NULL,
	role VARCHAR(20) NOT NULL,
	status VARCHAR(20) NOT NULL,
	joined_at TIMESTAMP WITH TIME ZONE NOT NULL,
	removed_at TIMESTAMP WITH TIME ZONE,
	PRIMARY KEY (id),
	CONSTRAINT ck_idn_membership_role CHECK (role IN ('owner', 'editor', 'viewer')),
	CONSTRAINT ck_idn_membership_status CHECK (status IN ('active', 'removed')),
	CONSTRAINT uq_idn_membership_workspace_user UNIQUE (workspace_id, user_id),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id),
	FOREIGN KEY(user_id) REFERENCES idn_user_accounts (id)
);

CREATE TABLE med_media_objects (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	kind VARCHAR(30) NOT NULL,
	source_type VARCHAR(30) NOT NULL,
	status VARCHAR(20) NOT NULL,
	current_version_id UUID,
	revision INTEGER NOT NULL,
	archived_at TIMESTAMP WITH TIME ZONE,
	archived_by UUID,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_med_media_object_kind CHECK (kind IN ('image', 'video', 'audio', 'subtitle', 'delivery', 'document')),
	CONSTRAINT ck_med_media_object_source CHECK (source_type IN ('upload', 'generated', 'rendered')),
	CONSTRAINT ck_med_media_object_status CHECK (status IN ('active', 'archived')),
	CONSTRAINT ck_med_media_object_revision CHECK (revision >= 1),
	CONSTRAINT uq_med_object_id_workspace UNIQUE (id, workspace_id),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id),
	FOREIGN KEY(archived_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_med_object_workspace_kind_status_created ON med_media_objects (workspace_id, kind, status, created_at);

CREATE TABLE med_upload_sessions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	media_object_id UUID,
	expected_current_version_id UUID,
	storage_profile VARCHAR(80) NOT NULL,
	bucket VARCHAR(255) NOT NULL,
	object_key VARCHAR(1024) NOT NULL,
	filename VARCHAR(255) NOT NULL,
	declared_kind VARCHAR(30) NOT NULL,
	declared_size_bytes BIGINT NOT NULL,
	declared_mime_type VARCHAR(120) NOT NULL,
	declared_sha256 VARCHAR(64) NOT NULL,
	status VARCHAR(20) NOT NULL,
	expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	completed_version_id UUID,
	completed_probe_task_id UUID,
	error_code VARCHAR(80),
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_med_upload_kind CHECK (declared_kind IN ('image', 'video', 'audio', 'subtitle', 'delivery', 'document')),
	CONSTRAINT ck_med_upload_status CHECK (status IN ('pending', 'completed', 'expired', 'failed')),
	CONSTRAINT ck_med_upload_size CHECK (declared_size_bytes >= 1),
	CONSTRAINT uq_med_upload_idempotency UNIQUE (workspace_id, idempotency_key),
	CONSTRAINT uq_med_upload_physical_object UNIQUE (storage_profile, bucket, object_key),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_med_upload_workspace_status_expiry ON med_upload_sessions (workspace_id, status, expires_at);

CREATE TABLE prod_tasks (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	task_type VARCHAR(50) NOT NULL,
	request_type VARCHAR(50) NOT NULL,
	request_id UUID NOT NULL,
	episode_id UUID,
	render_snapshot_id UUID,
	usage_type VARCHAR(50),
	usage_id UUID,
	input_version_id UUID,
	input_hash VARCHAR(64),
	status VARCHAR(30) NOT NULL,
	progress_stage VARCHAR(50) NOT NULL,
	error_code VARCHAR(80),
	error_retryable BOOLEAN,
	error_summary TEXT,
	next_action VARCHAR(80),
	cancel_status VARCHAR(20) NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	requested_by UUID NOT NULL,
	revision INTEGER NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_prod_task_status CHECK (status IN ('queued', 'running', 'waiting_provider', 'succeeded', 'failed', 'cancelled', 'unknown')),
	CONSTRAINT ck_prod_task_cancel_status CHECK (cancel_status IN ('none', 'requested', 'accepted', 'rejected')),
	CONSTRAINT ck_prod_task_revision CHECK (revision >= 1),
	CONSTRAINT uq_prod_task_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_prod_task_idempotency UNIQUE (workspace_id, task_type, idempotency_key),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id),
	FOREIGN KEY(requested_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_prod_task_workspace_status_created ON prod_tasks (workspace_id, status, created_at);

CREATE TABLE sys_outbox_events (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	event_type VARCHAR(100) NOT NULL,
	schema_version INTEGER NOT NULL,
	aggregate_type VARCHAR(50) NOT NULL,
	aggregate_id UUID NOT NULL,
	topic VARCHAR(100) NOT NULL,
	payload JSONB NOT NULL,
	trace_id VARCHAR(64) NOT NULL,
	traceparent VARCHAR(55) NOT NULL,
	causation_event_id UUID,
	status VARCHAR(30) NOT NULL,
	attempt_count INTEGER NOT NULL,
	available_at TIMESTAMP WITH TIME ZONE NOT NULL,
	claimed_at TIMESTAMP WITH TIME ZONE,
	claimed_by VARCHAR(100),
	published_at TIMESTAMP WITH TIME ZONE,
	last_error TEXT,
	occurred_at TIMESTAMP WITH TIME ZONE NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_sys_outbox_status CHECK (status IN ('pending', 'claimed', 'published', 'manual_attention')),
	CONSTRAINT ck_sys_outbox_schema_version CHECK (schema_version >= 1),
	CONSTRAINT ck_sys_outbox_attempt_count CHECK (attempt_count >= 0),
	CONSTRAINT uq_sys_outbox_aggregate_event UNIQUE (event_type, aggregate_id, schema_version),
	CONSTRAINT uq_sys_outbox_id_workspace UNIQUE (id, workspace_id),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id)
);

CREATE INDEX ix_sys_outbox_publishable ON sys_outbox_events (status, available_at);

CREATE TABLE sys_schedules (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	schedule_key VARCHAR(200) NOT NULL,
	handler_name VARCHAR(100) NOT NULL,
	scope JSONB NOT NULL,
	payload JSONB NOT NULL,
	kind VARCHAR(20) NOT NULL,
	rule JSONB NOT NULL,
	timezone VARCHAR(80) NOT NULL,
	status VARCHAR(30) NOT NULL,
	next_fire_at TIMESTAMP WITH TIME ZONE,
	next_attempt_at TIMESTAMP WITH TIME ZONE,
	misfire_policy VARCHAR(20) NOT NULL,
	max_catch_up INTEGER NOT NULL,
	failure_count INTEGER NOT NULL,
	last_error TEXT,
	revision INTEGER NOT NULL,
	lease_until TIMESTAMP WITH TIME ZONE,
	leased_by VARCHAR(100),
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_sys_schedule_kind CHECK (kind IN ('one_off', 'interval', 'cron')),
	CONSTRAINT ck_sys_schedule_status CHECK (status IN ('active', 'paused', 'completed', 'manual_attention')),
	CONSTRAINT ck_sys_schedule_misfire_policy CHECK (misfire_policy IN ('skip', 'run_once', 'catch_up')),
	CONSTRAINT ck_sys_schedule_max_catch_up CHECK (max_catch_up >= 0),
	CONSTRAINT ck_sys_schedule_failure_count CHECK (failure_count >= 0),
	CONSTRAINT ck_sys_schedule_revision CHECK (revision >= 1),
	CONSTRAINT uq_sys_schedule_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_sys_schedule_workspace_key UNIQUE (workspace_id, schedule_key),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_sys_schedule_due ON sys_schedules (status, next_fire_at, next_attempt_at);

CREATE TABLE prj_projects (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	name VARCHAR(120) NOT NULL,
	description TEXT,
	aspect_ratio VARCHAR(10) NOT NULL,
	language VARCHAR(35) NOT NULL,
	visual_style VARCHAR(200),
	target_duration_ms INTEGER NOT NULL,
	budget_limit NUMERIC(20, 6) NOT NULL,
	currency VARCHAR(3) NOT NULL,
	status VARCHAR(20) NOT NULL,
	revision INTEGER NOT NULL,
	archived_at TIMESTAMP WITH TIME ZONE,
	archived_by UUID,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_prj_project_status CHECK (status IN ('active', 'archived')),
	CONSTRAINT ck_prj_project_revision CHECK (revision >= 1),
	CONSTRAINT ck_prj_project_duration CHECK (target_duration_ms > 0),
	CONSTRAINT ck_prj_project_budget CHECK (budget_limit >= 0),
	CONSTRAINT uq_prj_project_id_workspace UNIQUE (id, workspace_id),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id),
	FOREIGN KEY(archived_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_prj_project_workspace_status_updated ON prj_projects (workspace_id, status, updated_at);

CREATE TABLE gov_consents (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	subject_identity JSONB NOT NULL,
	subject_type VARCHAR(40) NOT NULL,
	subject_id UUID NOT NULL,
	status VARCHAR(20) NOT NULL,
	current_revision_id UUID,
	revision INTEGER NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_gov_consent_status CHECK (status IN ('active', 'expired', 'revoked')),
	CONSTRAINT ck_gov_consent_subject_type CHECK (subject_type IN ('SCRIPT_VERSION', 'ASSET_VERSION', 'SHOT_SPEC_VERSION', 'CANDIDATE', 'MEDIA_VERSION', 'TIMELINE_VERSION', 'DELIVERY')),
	CONSTRAINT ck_gov_consent_revision CHECK (revision >= 1),
	CONSTRAINT uq_gov_consent_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_gov_consent_workspace_idempotency UNIQUE (workspace_id, idempotency_key),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_gov_consent_workspace_subject ON gov_consents (workspace_id, subject_type, subject_id);

CREATE INDEX ix_gov_consent_workspace_status_updated ON gov_consents (workspace_id, status, updated_at);

CREATE TABLE prod_provider_connections (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	preset_id VARCHAR(100) NOT NULL,
	catalog_version INTEGER NOT NULL,
	display_name VARCHAR(200) NOT NULL,
	protocol VARCHAR(40) NOT NULL,
	region VARCHAR(100),
	base_url VARCHAR(2048) NOT NULL,
	non_secret_config JSONB NOT NULL,
	configuration_status VARCHAR(20) NOT NULL,
	revision INTEGER NOT NULL,
	created_by UUID NOT NULL,
	updated_by UUID NOT NULL,
	archived_at TIMESTAMP WITH TIME ZONE,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_prod_provider_connection_catalog CHECK (catalog_version >= 1),
	CONSTRAINT ck_prod_provider_connection_revision CHECK (revision >= 1),
	CONSTRAINT ck_prod_provider_connection_protocol CHECK (protocol IN ('openai_compatible', 'anthropic_native', 'gemini_native', 'ark_native')),
	CONSTRAINT ck_prod_provider_connection_configuration CHECK (configuration_status IN ('incomplete', 'valid', 'invalid')),
	CONSTRAINT uq_prod_provider_connection_id_workspace UNIQUE (id, workspace_id),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id),
	FOREIGN KEY(updated_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_prod_provider_connection_workspace_archived ON prod_provider_connections (workspace_id, archived_at);

CREATE TABLE ast_assets (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	kind VARCHAR(30) NOT NULL,
	name VARCHAR(200) NOT NULL,
	normalized_name VARCHAR(200) NOT NULL,
	aliases TEXT[] NOT NULL,
	tags TEXT[] NOT NULL,
	status VARCHAR(20) NOT NULL,
	availability VARCHAR(20) NOT NULL,
	name_revision INTEGER NOT NULL,
	revision INTEGER NOT NULL,
	command_receipts JSONB NOT NULL,
	archived_at TIMESTAMP WITH TIME ZONE,
	archived_by UUID,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_ast_asset_project_workspace FOREIGN KEY(project_id, workspace_id) REFERENCES prj_projects (id, workspace_id),
	CONSTRAINT ck_ast_asset_kind CHECK (kind IN ('character', 'location', 'prop', 'costume', 'visual_style', 'voice')),
	CONSTRAINT ck_ast_asset_status CHECK (status IN ('active', 'archived')),
	CONSTRAINT ck_ast_asset_availability CHECK (availability IN ('enabled', 'disabled')),
	CONSTRAINT ck_ast_asset_revision CHECK (revision >= 1),
	CONSTRAINT ck_ast_asset_name_revision CHECK (name_revision >= 1),
	CONSTRAINT uq_ast_asset_id_workspace UNIQUE (id, workspace_id),
	FOREIGN KEY(archived_by) REFERENCES idn_user_accounts (id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_ast_asset_project_availability ON ast_assets (project_id, availability);

CREATE INDEX ix_ast_asset_project_kind_status ON ast_assets (project_id, kind, status);

CREATE INDEX ix_ast_asset_project_kind_normalized_name ON ast_assets (project_id, kind, normalized_name);

CREATE TABLE med_media_versions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	media_object_id UUID NOT NULL,
	version_no INTEGER NOT NULL,
	filename VARCHAR(255) NOT NULL,
	sha256 VARCHAR(64) NOT NULL,
	size_bytes BIGINT NOT NULL,
	mime_type VARCHAR(120) NOT NULL,
	probe_status VARCHAR(20) NOT NULL,
	probe_attempt INTEGER NOT NULL,
	probe_task_id UUID,
	probe_idempotency_key VARCHAR(200),
	probe_error_code VARCHAR(80),
	probe_error_summary TEXT,
	probe_next_action VARCHAR(80),
	width INTEGER,
	height INTEGER,
	duration_ms BIGINT,
	codec VARCHAR(120),
	container VARCHAR(120),
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_med_version_object_workspace FOREIGN KEY(media_object_id, workspace_id) REFERENCES med_media_objects (id, workspace_id) DEFERRABLE INITIALLY DEFERRED,
	CONSTRAINT ck_med_version_number CHECK (version_no >= 1),
	CONSTRAINT ck_med_version_size CHECK (size_bytes >= 1),
	CONSTRAINT ck_med_version_probe_attempt CHECK (probe_attempt >= 1),
	CONSTRAINT ck_med_version_probe_status CHECK (probe_status IN ('pending', 'ready', 'failed', 'quarantined')),
	CONSTRAINT uq_med_version_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_med_version_object_number UNIQUE (media_object_id, version_no),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_med_version_workspace_created ON med_media_versions (workspace_id, created_at);

CREATE INDEX ix_med_version_sha256 ON med_media_versions (sha256);

CREATE TABLE prod_attempts (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	task_id UUID NOT NULL,
	sequence INTEGER NOT NULL,
	provider_request_key VARCHAR(64) NOT NULL,
	provider_task_id VARCHAR(200),
	status VARCHAR(30) NOT NULL,
	request_snapshot_hash VARCHAR(64) NOT NULL,
	error_code VARCHAR(80),
	reconcile_summary TEXT,
	prepared_at TIMESTAMP WITH TIME ZONE NOT NULL,
	submitted_at TIMESTAMP WITH TIME ZONE,
	completed_at TIMESTAMP WITH TIME ZONE,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_prod_attempt_task_workspace FOREIGN KEY(task_id, workspace_id) REFERENCES prod_tasks (id, workspace_id),
	CONSTRAINT ck_prod_attempt_sequence CHECK (sequence >= 1),
	CONSTRAINT ck_prod_attempt_status CHECK (status IN ('prepared', 'submitting', 'accepted', 'polling', 'succeeded', 'failed', 'cancelled', 'unknown')),
	CONSTRAINT uq_prod_attempt_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_prod_attempt_task_sequence UNIQUE (task_id, sequence),
	CONSTRAINT uq_prod_attempt_provider_request_key UNIQUE (provider_request_key)
);

CREATE INDEX ix_prod_attempt_workspace_status ON prod_attempts (workspace_id, status);

CREATE TABLE sys_inbox_deliveries (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	event_id UUID NOT NULL,
	event_type VARCHAR(100) NOT NULL,
	consumer_name VARCHAR(100) NOT NULL,
	task_id UUID,
	trace_id VARCHAR(64) NOT NULL,
	status VARCHAR(30) NOT NULL,
	attempt_count INTEGER NOT NULL,
	received_at TIMESTAMP WITH TIME ZONE NOT NULL,
	processed_at TIMESTAMP WITH TIME ZONE,
	last_error VARCHAR(80),
	PRIMARY KEY (id),
	CONSTRAINT ck_sys_inbox_status CHECK (status IN ('processing', 'completed', 'rejected', 'retry_scheduled', 'manual_attention')),
	CONSTRAINT ck_sys_inbox_attempt_count CHECK (attempt_count >= 1),
	CONSTRAINT fk_sys_inbox_task_workspace FOREIGN KEY(task_id, workspace_id) REFERENCES prod_tasks (id, workspace_id),
	CONSTRAINT uq_sys_inbox_event_consumer UNIQUE (event_id, consumer_name),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id)
);

CREATE INDEX ix_sys_inbox_status_received ON sys_inbox_deliveries (status, received_at);

CREATE TABLE sys_schedule_fires (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	schedule_id UUID NOT NULL,
	fire_key VARCHAR(200) NOT NULL,
	scheduled_for TIMESTAMP WITH TIME ZONE NOT NULL,
	trigger_kind VARCHAR(20) NOT NULL,
	status VARCHAR(20) NOT NULL,
	task_id UUID NOT NULL,
	outbox_event_id UUID NOT NULL,
	trace_id VARCHAR(64) NOT NULL,
	error_summary TEXT,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT ck_sys_schedule_fire_trigger_kind CHECK (trigger_kind IN ('scheduled', 'manual')),
	CONSTRAINT ck_sys_schedule_fire_status CHECK (status IN ('dispatched')),
	CONSTRAINT fk_sys_schedule_fire_schedule_workspace FOREIGN KEY(schedule_id, workspace_id) REFERENCES sys_schedules (id, workspace_id),
	CONSTRAINT fk_sys_schedule_fire_task_workspace FOREIGN KEY(task_id, workspace_id) REFERENCES prod_tasks (id, workspace_id),
	CONSTRAINT fk_sys_schedule_fire_outbox_workspace FOREIGN KEY(outbox_event_id, workspace_id) REFERENCES sys_outbox_events (id, workspace_id),
	CONSTRAINT uq_sys_schedule_fire_key UNIQUE (schedule_id, fire_key)
);

CREATE INDEX ix_sys_schedule_fire_schedule_created ON sys_schedule_fires (schedule_id, created_at);

CREATE TABLE prj_episodes (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	name VARCHAR(120) NOT NULL,
	position INTEGER NOT NULL,
	target_duration_ms INTEGER NOT NULL,
	status VARCHAR(20) NOT NULL,
	revision INTEGER NOT NULL,
	current_script_version_id UUID,
	current_timeline_version_id UUID,
	archived_at TIMESTAMP WITH TIME ZONE,
	archived_by UUID,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_prj_episode_project_workspace FOREIGN KEY(project_id, workspace_id) REFERENCES prj_projects (id, workspace_id),
	CONSTRAINT ck_prj_episode_status CHECK (status IN ('active', 'archived')),
	CONSTRAINT ck_prj_episode_revision CHECK (revision >= 1),
	CONSTRAINT ck_prj_episode_position CHECK (position >= 1),
	CONSTRAINT ck_prj_episode_duration CHECK (target_duration_ms > 0),
	CONSTRAINT uq_prj_episode_id_workspace UNIQUE (id, workspace_id),
	FOREIGN KEY(archived_by) REFERENCES idn_user_accounts (id)
);

CREATE UNIQUE INDEX uq_prj_episode_active_position ON prj_episodes (project_id, position) WHERE status = 'active';

CREATE INDEX ix_prj_episode_project_status_position ON prj_episodes (project_id, status, position);

CREATE TABLE gov_consent_revisions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	consent_id UUID NOT NULL,
	revision_no INTEGER NOT NULL,
	action VARCHAR(20) NOT NULL,
	scope JSONB NOT NULL,
	valid_from TIMESTAMP WITH TIME ZONE NOT NULL,
	valid_to TIMESTAMP WITH TIME ZONE NOT NULL,
	reason TEXT NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_gov_revision_consent_workspace FOREIGN KEY(consent_id, workspace_id) REFERENCES gov_consents (id, workspace_id) DEFERRABLE INITIALLY DEFERRED,
	CONSTRAINT ck_gov_revision_number CHECK (revision_no >= 1),
	CONSTRAINT ck_gov_revision_action CHECK (action IN ('register', 'update', 'revoke')),
	CONSTRAINT ck_gov_revision_validity CHECK (valid_to > valid_from),
	CONSTRAINT uq_gov_revision_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_gov_revision_consent_number UNIQUE (consent_id, revision_no),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_gov_revision_consent_created ON gov_consent_revisions (consent_id, created_at);

CREATE TABLE prod_provider_credential_versions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	connection_id UUID NOT NULL,
	version INTEGER NOT NULL,
	key_id VARCHAR(100) NOT NULL,
	nonce BYTEA NOT NULL,
	ciphertext BYTEA NOT NULL,
	auth_tag BYTEA NOT NULL,
	fingerprint_hmac VARCHAR(64) NOT NULL,
	status VARCHAR(20) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	retired_at TIMESTAMP WITH TIME ZONE,
	PRIMARY KEY (id),
	CONSTRAINT fk_prod_provider_credential_connection_workspace FOREIGN KEY(connection_id, workspace_id) REFERENCES prod_provider_connections (id, workspace_id),
	CONSTRAINT ck_prod_provider_credential_version CHECK (version >= 1),
	CONSTRAINT ck_prod_provider_credential_status CHECK (status IN ('current', 'retiring', 'revoked')),
	CONSTRAINT ck_prod_provider_credential_nonce CHECK (octet_length(nonce) = 12),
	CONSTRAINT ck_prod_provider_credential_ciphertext CHECK (octet_length(ciphertext) > 0),
	CONSTRAINT ck_prod_provider_credential_auth_tag CHECK (octet_length(auth_tag) = 16),
	CONSTRAINT ck_prod_provider_credential_fingerprint CHECK (char_length(fingerprint_hmac) = 64),
	CONSTRAINT uq_prod_provider_credential_identity UNIQUE (id, workspace_id, connection_id),
	CONSTRAINT uq_prod_provider_credential_connection_version UNIQUE (connection_id, version),
	CONSTRAINT uq_prod_provider_credential_connection_fingerprint UNIQUE (connection_id, fingerprint_hmac),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE UNIQUE INDEX uq_prod_provider_credential_current ON prod_provider_credential_versions (connection_id) WHERE status = 'current';

CREATE INDEX ix_prod_provider_credential_workspace_created ON prod_provider_credential_versions (workspace_id, created_at);

CREATE TABLE ast_asset_name_revisions (
	asset_id UUID NOT NULL,
	revision_no INTEGER NOT NULL,
	workspace_id UUID NOT NULL,
	name_snapshot VARCHAR(200) NOT NULL,
	normalized_name VARCHAR(200) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (asset_id, revision_no),
	CONSTRAINT fk_ast_name_revision_asset_workspace FOREIGN KEY(asset_id, workspace_id) REFERENCES ast_assets (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT ck_ast_name_revision_number CHECK (revision_no >= 1),
	CONSTRAINT uq_ast_name_revision_scope UNIQUE (asset_id, workspace_id, revision_no),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_ast_name_revision_asset_created ON ast_asset_name_revisions (asset_id, created_at);

CREATE TABLE ast_asset_states (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	asset_id UUID NOT NULL,
	state_key VARCHAR(80) NOT NULL,
	label VARCHAR(120) NOT NULL,
	description TEXT NOT NULL,
	status VARCHAR(20) NOT NULL,
	current_version_id UUID,
	revision INTEGER NOT NULL,
	creation_key VARCHAR(200) NOT NULL,
	command_receipts JSONB NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_ast_state_asset_workspace FOREIGN KEY(asset_id, workspace_id) REFERENCES ast_assets (id, workspace_id),
	CONSTRAINT ck_ast_state_status CHECK (status IN ('active', 'disabled')),
	CONSTRAINT ck_ast_state_revision CHECK (revision >= 1),
	CONSTRAINT uq_ast_state_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_ast_state_scope UNIQUE (id, asset_id, workspace_id),
	CONSTRAINT uq_ast_state_asset_key UNIQUE (asset_id, state_key),
	CONSTRAINT uq_ast_state_asset_creation UNIQUE (asset_id, creation_key),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_ast_state_asset_status ON ast_asset_states (asset_id, status);

CREATE TABLE med_media_locations (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	media_version_id UUID NOT NULL,
	storage_profile VARCHAR(80) NOT NULL,
	bucket VARCHAR(255) NOT NULL,
	object_key VARCHAR(1024) NOT NULL,
	status VARCHAR(20) NOT NULL,
	verified_at TIMESTAMP WITH TIME ZONE,
	migration_task_id UUID,
	retire_after TIMESTAMP WITH TIME ZONE,
	retired_at TIMESTAMP WITH TIME ZONE,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_med_location_version_workspace FOREIGN KEY(media_version_id, workspace_id) REFERENCES med_media_versions (id, workspace_id) DEFERRABLE INITIALLY DEFERRED,
	CONSTRAINT ck_med_location_status CHECK (status IN ('verified', 'active', 'retiring', 'retired', 'quarantined')),
	CONSTRAINT uq_med_location_physical_object UNIQUE (storage_profile, bucket, object_key)
);

CREATE UNIQUE INDEX uq_med_location_active_version ON med_media_locations (media_version_id) WHERE status = 'active';

CREATE TABLE med_media_lineages (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	media_version_id UUID NOT NULL,
	source_type VARCHAR(40) NOT NULL,
	source_id UUID NOT NULL,
	source_hash VARCHAR(64) NOT NULL,
	position INTEGER NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_med_lineage_version_workspace FOREIGN KEY(media_version_id, workspace_id) REFERENCES med_media_versions (id, workspace_id),
	CONSTRAINT ck_med_lineage_position CHECK (position >= 1),
	CONSTRAINT ck_med_lineage_source_type CHECK (source_type IN ('asset_version', 'narrative_unit_version', 'script_version', 'shot_spec_version', 'storyboard_coverage', 'storyboard_export_snapshot', 'storyboard_readiness')),
	CONSTRAINT uq_med_lineage_version_position UNIQUE (media_version_id, position),
	CONSTRAINT uq_med_lineage_source UNIQUE (media_version_id, source_type, source_id)
);

CREATE INDEX ix_med_lineage_source ON med_media_lineages (source_type, source_id);

CREATE TABLE gov_consent_proofs (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	consent_revision_id UUID NOT NULL,
	media_version_id UUID NOT NULL,
	purpose VARCHAR(40) NOT NULL,
	position INTEGER NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_gov_proof_revision_workspace FOREIGN KEY(consent_revision_id, workspace_id) REFERENCES gov_consent_revisions (id, workspace_id),
	CONSTRAINT fk_gov_proof_media_workspace FOREIGN KEY(media_version_id, workspace_id) REFERENCES med_media_versions (id, workspace_id),
	CONSTRAINT ck_gov_proof_position CHECK (position >= 1),
	CONSTRAINT uq_gov_proof_revision_media UNIQUE (consent_revision_id, media_version_id),
	CONSTRAINT uq_gov_proof_revision_position UNIQUE (consent_revision_id, position)
);

CREATE INDEX ix_gov_proof_media ON gov_consent_proofs (media_version_id);

CREATE TABLE prod_provider_bindings (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	usage_type VARCHAR(40) NOT NULL,
	connection_id UUID NOT NULL,
	credential_version_id UUID NOT NULL,
	capability_id UUID NOT NULL,
	capability_config_version INTEGER NOT NULL,
	binding_revision INTEGER NOT NULL,
	status VARCHAR(20) NOT NULL,
	activated_by UUID NOT NULL,
	activated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	deactivated_by UUID,
	deactivated_at TIMESTAMP WITH TIME ZONE,
	PRIMARY KEY (id),
	CONSTRAINT fk_prod_provider_binding_credential_identity FOREIGN KEY(credential_version_id, workspace_id, connection_id) REFERENCES prod_provider_credential_versions (id, workspace_id, connection_id),
	CONSTRAINT fk_prod_provider_binding_capability_version FOREIGN KEY(capability_id, capability_config_version) REFERENCES prod_model_capabilities (id, config_version),
	CONSTRAINT ck_prod_provider_binding_usage CHECK (usage_type IN ('script_structure', 'image_generation', 'video_generation')),
	CONSTRAINT ck_prod_provider_binding_status CHECK (status IN ('active', 'inactive')),
	CONSTRAINT ck_prod_provider_binding_revision CHECK (binding_revision >= 1),
	CONSTRAINT ck_prod_provider_binding_capability_version CHECK (capability_config_version >= 1),
	CONSTRAINT ck_prod_provider_binding_lifecycle CHECK ((status = 'active' AND deactivated_by IS NULL AND deactivated_at IS NULL) OR (status = 'inactive' AND deactivated_by IS NOT NULL AND deactivated_at IS NOT NULL)),
	CONSTRAINT uq_prod_provider_binding_id_workspace UNIQUE (id, workspace_id),
	FOREIGN KEY(activated_by) REFERENCES idn_user_accounts (id),
	FOREIGN KEY(deactivated_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_prod_provider_binding_connection_status ON prod_provider_bindings (connection_id, status);

CREATE UNIQUE INDEX uq_prod_provider_binding_active_usage ON prod_provider_bindings (workspace_id, usage_type) WHERE status = 'active';

CREATE TABLE prod_provider_health_checks (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	connection_id UUID NOT NULL,
	connection_revision INTEGER NOT NULL,
	credential_version_id UUID NOT NULL,
	probe_type VARCHAR(40) NOT NULL,
	status VARCHAR(20) NOT NULL,
	latency_ms INTEGER,
	safe_error_code VARCHAR(80),
	checked_by UUID NOT NULL,
	checked_at TIMESTAMP WITH TIME ZONE NOT NULL,
	expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_prod_provider_health_credential_identity FOREIGN KEY(credential_version_id, workspace_id, connection_id) REFERENCES prod_provider_credential_versions (id, workspace_id, connection_id),
	CONSTRAINT ck_prod_provider_health_connection_revision CHECK (connection_revision >= 1),
	CONSTRAINT ck_prod_provider_health_probe_type CHECK (probe_type IN ('model_discovery', 'metadata')),
	CONSTRAINT ck_prod_provider_health_status CHECK (status IN ('healthy', 'degraded', 'unreachable')),
	CONSTRAINT ck_prod_provider_health_latency CHECK (latency_ms IS NULL OR latency_ms >= 0),
	CONSTRAINT ck_prod_provider_health_expiry CHECK (expires_at > checked_at),
	FOREIGN KEY(checked_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_prod_provider_health_connection_checked ON prod_provider_health_checks (connection_id, checked_at);

CREATE TABLE scr_script_documents (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	title VARCHAR(120) NOT NULL,
	source_type VARCHAR(20) NOT NULL,
	source_media_version_id UUID,
	language VARCHAR(35) NOT NULL,
	rights_declaration TEXT NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	status VARCHAR(20) NOT NULL,
	revision INTEGER NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_document_project_workspace FOREIGN KEY(project_id, workspace_id) REFERENCES prj_projects (id, workspace_id),
	CONSTRAINT fk_scr_document_media_workspace FOREIGN KEY(source_media_version_id, workspace_id) REFERENCES med_media_versions (id, workspace_id),
	CONSTRAINT ck_scr_document_source_type CHECK (source_type IN ('text', 'media')),
	CONSTRAINT ck_scr_document_source_reference CHECK ((source_type = 'text' AND source_media_version_id IS NULL) OR (source_type = 'media' AND source_media_version_id IS NOT NULL)),
	CONSTRAINT ck_scr_document_status CHECK (status IN ('active', 'archived')),
	CONSTRAINT ck_scr_document_revision CHECK (revision >= 1),
	CONSTRAINT uq_scr_document_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_document_project_idempotency UNIQUE (project_id, idempotency_key),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_document_project_status_created ON scr_script_documents (project_id, status, created_at);

CREATE TABLE scr_script_sources (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	input_type VARCHAR(20) NOT NULL,
	title VARCHAR(120) NOT NULL,
	source_media_version_id UUID,
	rights_declaration TEXT NOT NULL,
	status VARCHAR(20) NOT NULL,
	revision INTEGER NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	archived_at TIMESTAMP WITH TIME ZONE,
	archived_by UUID,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_source_episode_workspace FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT ck_scr_source_input_type CHECK (input_type IN ('text', 'media')),
	CONSTRAINT ck_scr_source_status CHECK (status IN ('active', 'archived')),
	CONSTRAINT ck_scr_source_revision CHECK (revision >= 1),
	CONSTRAINT uq_scr_source_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_source_episode_idempotency UNIQUE (episode_id, idempotency_key),
	FOREIGN KEY(archived_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_source_episode_status ON scr_script_sources (episode_id, status);

CREATE TABLE scr_narrative_units (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	kind VARCHAR(30) NOT NULL,
	status VARCHAR(20) NOT NULL,
	current_version_id UUID,
	revision INTEGER NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_narrative_unit_episode_workspace FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT ck_scr_narrative_unit_kind CHECK (kind IN ('scene_heading', 'action', 'dialogue', 'narration')),
	CONSTRAINT ck_scr_narrative_unit_status CHECK (status IN ('active', 'retired')),
	CONSTRAINT ck_scr_narrative_unit_revision CHECK (revision >= 1),
	CONSTRAINT uq_scr_narrative_unit_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_narrative_unit_scope UNIQUE (id, episode_id, workspace_id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_narrative_unit_episode_status ON scr_narrative_units (episode_id, status);

CREATE TABLE sbd_shot_transforms (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	operation VARCHAR(20) NOT NULL,
	source_shot_ids UUID[] NOT NULL,
	source_spec_version_ids UUID[] NOT NULL,
	result_shot_ids UUID[] NOT NULL,
	impact_hash VARCHAR(64) NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	actor_id UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_transform_episode_workspace FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT ck_sbd_transform_operation CHECK (operation IN ('copy', 'split', 'merge')),
	CONSTRAINT uq_sbd_transform_workspace_idempotency UNIQUE (workspace_id, idempotency_key),
	FOREIGN KEY(workspace_id) REFERENCES idn_workspaces (id),
	FOREIGN KEY(actor_id) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_sbd_transform_input_hash ON sbd_shot_transforms (input_hash);

CREATE INDEX ix_sbd_transform_episode_created ON sbd_shot_transforms (episode_id, created_at);

CREATE TABLE sbd_export_jobs (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	schema_version INTEGER NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	input_snapshot JSONB NOT NULL,
	command_hash VARCHAR(64) NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	task_id UUID,
	status VARCHAR(20) NOT NULL,
	error_code VARCHAR(80),
	error_summary TEXT,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_export_job_project FOREIGN KEY(project_id, workspace_id) REFERENCES prj_projects (id, workspace_id),
	CONSTRAINT fk_sbd_export_job_episode FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT fk_sbd_export_job_task FOREIGN KEY(task_id, workspace_id) REFERENCES prod_tasks (id, workspace_id) DEFERRABLE INITIALLY DEFERRED,
	CONSTRAINT ck_sbd_export_job_status CHECK (status IN ('queued', 'running', 'succeeded', 'failed')),
	CONSTRAINT ck_sbd_export_job_schema CHECK (schema_version = 1),
	CONSTRAINT uq_sbd_export_job_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_sbd_export_job_scope UNIQUE (id, episode_id, workspace_id),
	CONSTRAINT uq_sbd_export_job_task UNIQUE (task_id),
	CONSTRAINT uq_sbd_export_job_idempotency UNIQUE (episode_id, idempotency_key),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_sbd_export_job_episode_created ON sbd_export_jobs (episode_id, created_at);

CREATE INDEX ix_sbd_export_job_workspace_status ON sbd_export_jobs (workspace_id, status);

CREATE TABLE ast_asset_versions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	asset_id UUID NOT NULL,
	asset_state_id UUID NOT NULL,
	version_no INTEGER NOT NULL,
	schema_version INTEGER NOT NULL,
	spec JSONB NOT NULL,
	prompt_description TEXT NOT NULL,
	source_type VARCHAR(30) NOT NULL,
	source_id UUID,
	content_hash VARCHAR(64) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_ast_version_state_scope FOREIGN KEY(asset_state_id, asset_id, workspace_id) REFERENCES ast_asset_states (id, asset_id, workspace_id) DEFERRABLE INITIALLY DEFERRED,
	CONSTRAINT ck_ast_version_number CHECK (version_no >= 1),
	CONSTRAINT ck_ast_schema_version CHECK (schema_version >= 1),
	CONSTRAINT ck_ast_version_source_type CHECK (source_type IN ('manual', 'script_extraction_candidate', 'production_bible_state')),
	CONSTRAINT uq_ast_version_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_ast_version_scope UNIQUE (id, asset_state_id, asset_id, workspace_id),
	CONSTRAINT uq_ast_version_number UNIQUE (asset_id, version_no),
	CONSTRAINT uq_ast_version_source UNIQUE (source_type, source_id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_ast_version_content_hash ON ast_asset_versions (content_hash);

CREATE INDEX ix_ast_version_state_number ON ast_asset_versions (asset_state_id, version_no);

CREATE TABLE scr_document_revisions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	document_id UUID NOT NULL,
	version_no INTEGER NOT NULL,
	source_type VARCHAR(20) NOT NULL,
	source_media_version_id UUID,
	raw_text TEXT NOT NULL,
	raw_hash VARCHAR(64) NOT NULL,
	normalized_text TEXT NOT NULL,
	normalized_hash VARCHAR(64) NOT NULL,
	normalizer_version VARCHAR(80) NOT NULL,
	normalization_map JSONB NOT NULL,
	codepoint_count INTEGER NOT NULL,
	analysis_status VARCHAR(30) NOT NULL,
	analyzer_version VARCHAR(80) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_document_revision_document_workspace FOREIGN KEY(document_id, workspace_id) REFERENCES scr_script_documents (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_scr_document_revision_media_workspace FOREIGN KEY(source_media_version_id, workspace_id) REFERENCES med_media_versions (id, workspace_id),
	CONSTRAINT ck_scr_document_revision_number CHECK (version_no >= 1),
	CONSTRAINT ck_scr_document_revision_source_type CHECK (source_type IN ('text', 'media')),
	CONSTRAINT ck_scr_document_revision_source_reference CHECK ((source_type = 'text' AND source_media_version_id IS NULL) OR (source_type = 'media' AND source_media_version_id IS NOT NULL)),
	CONSTRAINT ck_scr_document_revision_analysis_status CHECK (analysis_status IN ('deterministic', 'ai_candidate_required', 'rejected')),
	CONSTRAINT ck_scr_document_revision_codepoints CHECK (codepoint_count >= 1),
	CONSTRAINT uq_scr_document_revision_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_document_revision_number UNIQUE (document_id, version_no),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_document_revision_raw_hash ON scr_document_revisions (raw_hash);

CREATE INDEX ix_scr_document_revision_document_created ON scr_document_revisions (document_id, created_at);

CREATE TABLE scr_script_versions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	source_id UUID NOT NULL,
	version_no INTEGER NOT NULL,
	status VARCHAR(20) NOT NULL,
	body TEXT NOT NULL,
	content_hash VARCHAR(64) NOT NULL,
	structure_summary JSONB NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_version_source_workspace FOREIGN KEY(source_id, workspace_id) REFERENCES scr_script_sources (id, workspace_id),
	CONSTRAINT ck_scr_version_number CHECK (version_no >= 1),
	CONSTRAINT ck_scr_version_status CHECK (status IN ('draft', 'published')),
	CONSTRAINT uq_scr_version_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_version_number UNIQUE (source_id, version_no),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_version_content_hash ON scr_script_versions (content_hash);

CREATE TABLE sbd_export_manifests (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	job_id UUID NOT NULL,
	schema_version INTEGER NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	input_snapshot JSONB NOT NULL,
	file_manifest JSONB NOT NULL,
	media_version_id UUID NOT NULL,
	package_sha256 VARCHAR(64) NOT NULL,
	package_size_bytes BIGINT NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_export_manifest_job FOREIGN KEY(job_id, episode_id, workspace_id) REFERENCES sbd_export_jobs (id, episode_id, workspace_id),
	CONSTRAINT fk_sbd_export_manifest_media FOREIGN KEY(media_version_id, workspace_id) REFERENCES med_media_versions (id, workspace_id),
	CONSTRAINT ck_sbd_export_manifest_schema CHECK (schema_version = 1),
	CONSTRAINT ck_sbd_export_manifest_size CHECK (package_size_bytes >= 1),
	CONSTRAINT uq_sbd_export_manifest_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_sbd_export_manifest_job UNIQUE (job_id),
	CONSTRAINT uq_sbd_export_manifest_media UNIQUE (media_version_id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_sbd_export_manifest_episode_created ON sbd_export_manifests (episode_id, created_at);

CREATE TABLE ast_media_references (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	asset_version_id UUID NOT NULL,
	media_version_id UUID NOT NULL,
	purpose VARCHAR(40) NOT NULL,
	position INTEGER NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_ast_media_ref_version_workspace FOREIGN KEY(asset_version_id, workspace_id) REFERENCES ast_asset_versions (id, workspace_id),
	CONSTRAINT fk_ast_media_ref_media_workspace FOREIGN KEY(media_version_id, workspace_id) REFERENCES med_media_versions (id, workspace_id),
	CONSTRAINT ck_ast_media_ref_position CHECK (position >= 1),
	CONSTRAINT uq_ast_media_ref_purpose_position UNIQUE (asset_version_id, purpose, position)
);

CREATE INDEX ix_ast_media_ref_media ON ast_media_references (media_version_id);

CREATE TABLE scr_narrative_blocks (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	document_revision_id UUID NOT NULL,
	position INTEGER NOT NULL,
	kind VARCHAR(30) NOT NULL,
	source_start INTEGER NOT NULL,
	source_end INTEGER NOT NULL,
	text_hash VARCHAR(64) NOT NULL,
	block_metadata JSONB NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_narrative_block_revision_workspace FOREIGN KEY(document_revision_id, workspace_id) REFERENCES scr_document_revisions (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT ck_scr_narrative_block_position CHECK (position >= 1),
	CONSTRAINT ck_scr_narrative_block_kind CHECK (kind IN ('preamble', 'episode_marker', 'scene_heading', 'dialogue', 'narration', 'action', 'separator')),
	CONSTRAINT ck_scr_narrative_block_source_start CHECK (source_start >= 0),
	CONSTRAINT ck_scr_narrative_block_source_range CHECK (source_end > source_start),
	CONSTRAINT uq_scr_narrative_block_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_narrative_block_revision_position UNIQUE (document_revision_id, position)
);

CREATE INDEX ix_scr_narrative_block_revision_range ON scr_narrative_blocks (document_revision_id, source_start);

CREATE TABLE scr_format_issues (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	document_revision_id UUID NOT NULL,
	position INTEGER NOT NULL,
	code VARCHAR(80) NOT NULL,
	severity VARCHAR(20) NOT NULL,
	source_start INTEGER NOT NULL,
	source_end INTEGER NOT NULL,
	line_number INTEGER NOT NULL,
	column_number INTEGER NOT NULL,
	next_action VARCHAR(100) NOT NULL,
	issue_details JSONB NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_format_issue_revision_workspace FOREIGN KEY(document_revision_id, workspace_id) REFERENCES scr_document_revisions (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT ck_scr_format_issue_position CHECK (position >= 1),
	CONSTRAINT ck_scr_format_issue_severity CHECK (severity IN ('warning', 'blocking')),
	CONSTRAINT ck_scr_format_issue_source_start CHECK (source_start >= 0),
	CONSTRAINT ck_scr_format_issue_source_range CHECK (source_end > source_start),
	CONSTRAINT ck_scr_format_issue_line CHECK (line_number >= 1),
	CONSTRAINT ck_scr_format_issue_column CHECK (column_number >= 1),
	CONSTRAINT uq_scr_format_issue_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_format_issue_revision_position UNIQUE (document_revision_id, position)
);

CREATE INDEX ix_scr_format_issue_revision_severity ON scr_format_issues (document_revision_id, severity);

CREATE TABLE scr_episode_plans (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	document_revision_id UUID NOT NULL,
	strategy VARCHAR(40) NOT NULL,
	status VARCHAR(30) NOT NULL,
	target_duration_ms INTEGER NOT NULL,
	requested_episode_count INTEGER,
	total_estimated_duration_ms INTEGER NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	planning_engine_version VARCHAR(80) NOT NULL,
	model_name VARCHAR(160),
	prompt_version VARCHAR(80),
	schema_version VARCHAR(80) NOT NULL,
	planning_task_id UUID,
	planning_error_code VARCHAR(80),
	command_receipts JSONB NOT NULL,
	revision INTEGER NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	confirmed_by UUID,
	confirmed_at TIMESTAMP WITH TIME ZONE,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_episode_plan_revision_workspace FOREIGN KEY(document_revision_id, workspace_id) REFERENCES scr_document_revisions (id, workspace_id),
	CONSTRAINT fk_scr_episode_plan_project_workspace FOREIGN KEY(project_id, workspace_id) REFERENCES prj_projects (id, workspace_id),
	CONSTRAINT fk_scr_episode_plan_task_workspace FOREIGN KEY(planning_task_id, workspace_id) REFERENCES prod_tasks (id, workspace_id),
	CONSTRAINT ck_scr_episode_plan_strategy CHECK (strategy IN ('explicit_markers', 'target_duration_ai')),
	CONSTRAINT ck_scr_episode_plan_status CHECK (status IN ('draft', 'review_ready', 'confirmed', 'materialized', 'superseded')),
	CONSTRAINT ck_scr_episode_plan_revision CHECK (revision >= 1),
	CONSTRAINT ck_scr_episode_plan_duration CHECK (target_duration_ms >= 1000),
	CONSTRAINT ck_scr_episode_plan_requested_count CHECK (requested_episode_count IS NULL OR requested_episode_count >= 1),
	CONSTRAINT ck_scr_episode_plan_total_duration CHECK (total_estimated_duration_ms >= 0),
	CONSTRAINT uq_scr_episode_plan_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_episode_plan_project_idempotency UNIQUE (project_id, idempotency_key),
	FOREIGN KEY(confirmed_by) REFERENCES idn_user_accounts (id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_episode_plan_revision_status_created ON scr_episode_plans (document_revision_id, status, created_at);

CREATE TABLE scr_adaptation_runs (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	source_id UUID NOT NULL,
	input_script_version_id UUID NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	target_duration_ms INTEGER NOT NULL,
	core_plot_points JSONB NOT NULL,
	pacing VARCHAR(20) NOT NULL,
	colloquial_dialogue BOOLEAN NOT NULL,
	adaptation_engine_version VARCHAR(80) NOT NULL,
	model_name VARCHAR(160) NOT NULL,
	prompt_version VARCHAR(80) NOT NULL,
	schema_version VARCHAR(80) NOT NULL,
	task_id UUID,
	status VARCHAR(30) NOT NULL,
	candidate_body TEXT,
	candidate_hash VARCHAR(64),
	draft_body TEXT,
	draft_hash VARCHAR(64),
	change_summary TEXT,
	estimated_duration_ms INTEGER,
	error_code VARCHAR(80),
	published_script_version_id UUID,
	publish_idempotency_key VARCHAR(200),
	publish_command_hash VARCHAR(64),
	publish_result_snapshot JSONB NOT NULL,
	cancel_idempotency_key VARCHAR(200),
	cancel_command_hash VARCHAR(64),
	revision INTEGER NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_adaptation_episode_workspace FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT fk_scr_adaptation_source_workspace FOREIGN KEY(source_id, workspace_id) REFERENCES scr_script_sources (id, workspace_id),
	CONSTRAINT fk_scr_adaptation_input_workspace FOREIGN KEY(input_script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT fk_scr_adaptation_published_workspace FOREIGN KEY(published_script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT fk_scr_adaptation_task_workspace FOREIGN KEY(task_id, workspace_id) REFERENCES prod_tasks (id, workspace_id),
	CONSTRAINT ck_scr_adaptation_status CHECK (status IN ('queued', 'running', 'succeeded', 'published', 'failed', 'cancelled', 'unknown')),
	CONSTRAINT ck_scr_adaptation_target_duration CHECK (target_duration_ms >= 15000 AND target_duration_ms <= 600000),
	CONSTRAINT ck_scr_adaptation_pacing CHECK (pacing IN ('slow', 'balanced', 'fast')),
	CONSTRAINT ck_scr_adaptation_revision CHECK (revision >= 1),
	CONSTRAINT ck_scr_adaptation_estimated_duration CHECK (estimated_duration_ms IS NULL OR (estimated_duration_ms >= 1000 AND estimated_duration_ms <= 600000)),
	CONSTRAINT ck_scr_adaptation_candidate_pair CHECK ((candidate_body IS NULL) = (candidate_hash IS NULL)),
	CONSTRAINT ck_scr_adaptation_draft_pair CHECK ((draft_body IS NULL) = (draft_hash IS NULL)),
	CONSTRAINT ck_scr_adaptation_published_version CHECK (status != 'published' OR published_script_version_id IS NOT NULL),
	CONSTRAINT uq_scr_adaptation_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_adaptation_task UNIQUE (task_id),
	CONSTRAINT uq_scr_adaptation_episode_idempotency UNIQUE (episode_id, idempotency_key),
	CONSTRAINT uq_scr_adaptation_publish_idempotency UNIQUE (workspace_id, publish_idempotency_key),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_adaptation_episode_created ON scr_adaptation_runs (episode_id, created_at);

CREATE INDEX ix_scr_adaptation_workspace_status_created ON scr_adaptation_runs (workspace_id, status, created_at);

CREATE TABLE scr_scenes (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	script_version_id UUID NOT NULL,
	position INTEGER NOT NULL,
	heading VARCHAR(200) NOT NULL,
	location VARCHAR(200) NOT NULL,
	time_of_day VARCHAR(100) NOT NULL,
	summary TEXT NOT NULL,
	semantic_context JSONB NOT NULL,
	source_start INTEGER NOT NULL,
	source_end INTEGER NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_scene_version_workspace FOREIGN KEY(script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT ck_scr_scene_position CHECK (position >= 1),
	CONSTRAINT ck_scr_scene_source_start CHECK (source_start >= 0),
	CONSTRAINT ck_scr_scene_source_range CHECK (source_end > source_start),
	CONSTRAINT uq_scr_scene_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_scene_version_position UNIQUE (script_version_id, position)
);

CREATE INDEX ix_scr_scene_version_range ON scr_scenes (script_version_id, source_start);

CREATE TABLE scr_narrative_structures (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	script_version_id UUID NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	parser_version VARCHAR(80) NOT NULL,
	structure_hash VARCHAR(64) NOT NULL,
	dependency_hash VARCHAR(64) NOT NULL,
	revision INTEGER NOT NULL,
	command_receipts JSONB NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_narrative_structure_episode_workspace FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT fk_scr_narrative_structure_script_workspace FOREIGN KEY(script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT ck_scr_narrative_structure_revision CHECK (revision >= 1),
	CONSTRAINT uq_scr_narrative_structure_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_narrative_structure_scope UNIQUE (id, script_version_id, episode_id, workspace_id),
	CONSTRAINT uq_scr_narrative_structure_script UNIQUE (script_version_id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_narrative_structure_episode_created ON scr_narrative_structures (episode_id, created_at);

CREATE TABLE scr_narrative_impacts (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	sequence INTEGER NOT NULL,
	trigger VARCHAR(30) NOT NULL,
	episode_revision INTEGER NOT NULL,
	previous_script_version_id UUID,
	current_script_version_id UUID NOT NULL,
	previous_structure_hash VARCHAR(64),
	current_structure_hash VARCHAR(64) NOT NULL,
	previous_dependency_hash VARCHAR(64),
	current_dependency_hash VARCHAR(64) NOT NULL,
	previous_unit_count INTEGER NOT NULL,
	current_unit_count INTEGER NOT NULL,
	affected_shot_ids UUID[] NOT NULL,
	invalidated_scopes VARCHAR(40)[] NOT NULL,
	impact_hash VARCHAR(64) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_narrative_impact_episode_workspace FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT fk_scr_narrative_impact_previous_workspace FOREIGN KEY(previous_script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT fk_scr_narrative_impact_current_workspace FOREIGN KEY(current_script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT ck_scr_narrative_impact_sequence CHECK (sequence >= 1),
	CONSTRAINT ck_scr_narrative_impact_trigger CHECK (trigger IN ('current_changed', 'structure_corrected')),
	CONSTRAINT ck_scr_narrative_impact_episode_revision CHECK (episode_revision >= 1),
	CONSTRAINT ck_scr_narrative_impact_unit_counts CHECK (previous_unit_count >= 0 AND current_unit_count >= 0),
	CONSTRAINT uq_scr_narrative_impact_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_narrative_impact_episode_sequence UNIQUE (episode_id, sequence),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_narrative_impact_episode_created ON scr_narrative_impacts (episode_id, created_at);

CREATE TABLE scr_production_bibles (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	document_revision_id UUID NOT NULL,
	task_id UUID,
	status VARCHAR(30) NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	result_hash VARCHAR(64),
	engine_version VARCHAR(80) NOT NULL,
	model_name VARCHAR(160) NOT NULL,
	prompt_version VARCHAR(80) NOT NULL,
	schema_version VARCHAR(80) NOT NULL,
	harness_version VARCHAR(80) NOT NULL,
	checkpoint JSONB,
	checkpoint_revision INTEGER NOT NULL,
	checkpoint_updated_at TIMESTAMP WITH TIME ZONE,
	run_token UUID,
	lease_expires_at TIMESTAMP WITH TIME ZONE,
	review_issues JSONB NOT NULL,
	resume_receipts JSONB NOT NULL,
	review_receipts JSONB NOT NULL,
	revision INTEGER NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	confirm_idempotency_key VARCHAR(200),
	confirm_command_hash VARCHAR(64),
	confirm_result JSONB NOT NULL,
	confirmed_at TIMESTAMP WITH TIME ZONE,
	confirmed_by UUID,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_prod_bible_project_workspace FOREIGN KEY(project_id, workspace_id) REFERENCES prj_projects (id, workspace_id),
	CONSTRAINT fk_scr_prod_bible_document_revision_workspace FOREIGN KEY(document_revision_id, workspace_id) REFERENCES scr_document_revisions (id, workspace_id),
	CONSTRAINT fk_scr_prod_bible_task_workspace FOREIGN KEY(task_id, workspace_id) REFERENCES prod_tasks (id, workspace_id),
	CONSTRAINT ck_scr_prod_bible_status CHECK (status IN ('queued', 'running', 'needs_review', 'confirmed', 'failed', 'unknown', 'superseded', 'cancelled')),
	CONSTRAINT ck_scr_prod_bible_input_hash CHECK (char_length(input_hash) = 64),
	CONSTRAINT ck_scr_prod_bible_result_hash CHECK (result_hash IS NULL OR char_length(result_hash) = 64),
	CONSTRAINT ck_scr_prod_bible_checkpoint_revision CHECK (checkpoint_revision >= 0),
	CONSTRAINT ck_scr_prod_bible_revision CHECK (revision >= 1),
	CONSTRAINT ck_scr_prod_bible_checkpoint CHECK ((checkpoint IS NULL AND checkpoint_revision = 0 AND checkpoint_updated_at IS NULL) OR (checkpoint IS NOT NULL AND checkpoint_revision >= 1 AND checkpoint_updated_at IS NOT NULL)),
	CONSTRAINT ck_scr_prod_bible_lease CHECK ((run_token IS NULL AND lease_expires_at IS NULL) OR (run_token IS NOT NULL AND lease_expires_at IS NOT NULL)),
	CONSTRAINT ck_scr_prod_bible_resume_receipts CHECK (jsonb_typeof(resume_receipts) = 'object'),
	CONSTRAINT ck_scr_prod_bible_review_receipts CHECK (jsonb_typeof(review_receipts) = 'object'),
	CONSTRAINT ck_scr_prod_bible_confirmation_receipt CHECK ((confirmed_at IS NULL AND confirmed_by IS NULL AND confirm_idempotency_key IS NULL AND confirm_command_hash IS NULL) OR (confirmed_at IS NOT NULL AND confirmed_by IS NOT NULL AND confirm_idempotency_key IS NOT NULL AND confirm_command_hash IS NOT NULL AND char_length(confirm_command_hash) = 64)),
	CONSTRAINT uq_scr_prod_bible_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_prod_bible_scope UNIQUE (id, project_id, workspace_id),
	CONSTRAINT uq_scr_prod_bible_revision_idempotency UNIQUE (document_revision_id, idempotency_key),
	CONSTRAINT uq_scr_prod_bible_confirm_idempotency UNIQUE (workspace_id, confirm_idempotency_key),
	FOREIGN KEY(confirmed_by) REFERENCES idn_user_accounts (id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_prod_bible_project_status_created ON scr_production_bibles (project_id, status, created_at);

CREATE INDEX ix_scr_prod_bible_revision_status_created ON scr_production_bibles (document_revision_id, status, created_at);

CREATE UNIQUE INDEX uq_scr_prod_bible_project_confirmed ON scr_production_bibles (project_id) WHERE status = 'confirmed';

CREATE TABLE scr_episode_proposals (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	plan_id UUID NOT NULL,
	position INTEGER NOT NULL,
	title VARCHAR(120) NOT NULL,
	start_block_id UUID NOT NULL,
	end_block_id UUID NOT NULL,
	start_block_position INTEGER NOT NULL,
	end_block_position INTEGER NOT NULL,
	source_start INTEGER NOT NULL,
	source_end INTEGER NOT NULL,
	content_hash VARCHAR(64) NOT NULL,
	estimated_duration_ms INTEGER NOT NULL,
	reason TEXT NOT NULL,
	confidence NUMERIC(5, 4) NOT NULL,
	boundary_evidence JSONB NOT NULL,
	is_locked BOOLEAN NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_episode_proposal_plan_workspace FOREIGN KEY(plan_id, workspace_id) REFERENCES scr_episode_plans (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_scr_episode_proposal_start_block_workspace FOREIGN KEY(start_block_id, workspace_id) REFERENCES scr_narrative_blocks (id, workspace_id),
	CONSTRAINT fk_scr_episode_proposal_end_block_workspace FOREIGN KEY(end_block_id, workspace_id) REFERENCES scr_narrative_blocks (id, workspace_id),
	CONSTRAINT ck_scr_episode_proposal_position CHECK (position >= 1),
	CONSTRAINT ck_scr_episode_proposal_start_block_position CHECK (start_block_position >= 1),
	CONSTRAINT ck_scr_episode_proposal_block_range CHECK (end_block_position >= start_block_position),
	CONSTRAINT ck_scr_episode_proposal_source_start CHECK (source_start >= 0),
	CONSTRAINT ck_scr_episode_proposal_source_range CHECK (source_end > source_start),
	CONSTRAINT ck_scr_episode_proposal_duration CHECK (estimated_duration_ms >= 1000),
	CONSTRAINT ck_scr_episode_proposal_confidence CHECK (confidence >= 0 AND confidence <= 1),
	CONSTRAINT uq_scr_episode_proposal_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_episode_proposal_plan_position UNIQUE (plan_id, position)
);

CREATE INDEX ix_scr_episode_proposal_plan_range ON scr_episode_proposals (plan_id, source_start);

CREATE TABLE scr_import_commits (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	plan_id UUID NOT NULL,
	mode VARCHAR(30) NOT NULL,
	status VARCHAR(30) NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	expected_project_revision INTEGER NOT NULL,
	expected_active_order_hash VARCHAR(64) NOT NULL,
	result_snapshot JSONB NOT NULL,
	publish_input_hash VARCHAR(64),
	publish_idempotency_key VARCHAR(200),
	error_code VARCHAR(80),
	revision INTEGER NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_import_commit_plan_workspace FOREIGN KEY(plan_id, workspace_id) REFERENCES scr_episode_plans (id, workspace_id),
	CONSTRAINT fk_scr_import_commit_project_workspace FOREIGN KEY(project_id, workspace_id) REFERENCES prj_projects (id, workspace_id),
	CONSTRAINT ck_scr_import_commit_mode CHECK (mode IN ('append_new')),
	CONSTRAINT ck_scr_import_commit_status CHECK (status IN ('pending', 'materializing', 'materialized', 'publishing', 'published', 'conflict', 'failed')),
	CONSTRAINT ck_scr_import_commit_revision CHECK (revision >= 1),
	CONSTRAINT ck_scr_import_commit_project_revision CHECK (expected_project_revision >= 1),
	CONSTRAINT uq_scr_import_commit_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_import_commit_workspace_idempotency UNIQUE (workspace_id, idempotency_key),
	CONSTRAINT uq_scr_import_commit_publish_idempotency UNIQUE (workspace_id, publish_idempotency_key),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_import_commit_plan_created ON scr_import_commits (plan_id, created_at);

CREATE TABLE scr_dialogues (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	scene_id UUID NOT NULL,
	position INTEGER NOT NULL,
	speaker_candidate VARCHAR(200) NOT NULL,
	dialogue_kind VARCHAR(30) NOT NULL,
	text TEXT NOT NULL,
	performance_note TEXT,
	source_start INTEGER NOT NULL,
	source_end INTEGER NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_dialogue_scene_workspace FOREIGN KEY(scene_id, workspace_id) REFERENCES scr_scenes (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT ck_scr_dialogue_position CHECK (position >= 1),
	CONSTRAINT ck_scr_dialogue_kind CHECK (dialogue_kind IN ('spoken', 'narration', 'internal', 'voice_over')),
	CONSTRAINT ck_scr_dialogue_source_start CHECK (source_start >= 0),
	CONSTRAINT ck_scr_dialogue_source_range CHECK (source_end > source_start),
	CONSTRAINT uq_scr_dialogue_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_dialogue_scene_position UNIQUE (scene_id, position)
);

CREATE INDEX ix_scr_dialogue_scene_range ON scr_dialogues (scene_id, source_start);

CREATE TABLE scr_extraction_batches (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	script_version_id UUID NOT NULL,
	task_id UUID,
	scope VARCHAR(20) NOT NULL,
	extractor_version VARCHAR(80) NOT NULL,
	script_content_hash VARCHAR(64) NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	production_bible_id UUID,
	production_bible_revision INTEGER,
	production_bible_result_hash VARCHAR(64),
	status VARCHAR(30) NOT NULL,
	confirmed_script_version_id UUID,
	result_hash VARCHAR(64),
	candidate_count INTEGER NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_batch_version_workspace FOREIGN KEY(script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT fk_scr_batch_confirmed_version_workspace FOREIGN KEY(confirmed_script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT fk_scr_batch_task_workspace FOREIGN KEY(task_id, workspace_id) REFERENCES prod_tasks (id, workspace_id),
	CONSTRAINT fk_scr_batch_production_bible_workspace FOREIGN KEY(production_bible_id, workspace_id) REFERENCES scr_production_bibles (id, workspace_id),
	CONSTRAINT ck_scr_batch_scope CHECK (scope = 'full'),
	CONSTRAINT ck_scr_batch_status CHECK (status IN ('queued', 'running', 'waiting_provider', 'succeeded', 'failed', 'cancelled', 'unknown')),
	CONSTRAINT ck_scr_batch_candidate_count CHECK (candidate_count >= 0),
	CONSTRAINT ck_scr_batch_production_bible_snapshot CHECK ((production_bible_id IS NULL AND production_bible_revision IS NULL AND production_bible_result_hash IS NULL) OR (production_bible_id IS NOT NULL AND production_bible_revision >= 1 AND char_length(production_bible_result_hash) = 64)),
	CONSTRAINT uq_scr_batch_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_batch_version_idempotency UNIQUE (script_version_id, idempotency_key),
	CONSTRAINT uq_scr_batch_task UNIQUE (task_id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_batch_workspace_status_created ON scr_extraction_batches (workspace_id, status, created_at);

CREATE TABLE scr_production_bible_entities (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	bible_id UUID NOT NULL,
	entity_key VARCHAR(100) NOT NULL,
	kind VARCHAR(30) NOT NULL,
	canonical_name VARCHAR(200) NOT NULL,
	normalized_name VARCHAR(200) NOT NULL,
	aliases TEXT[] NOT NULL,
	stable_spec JSONB NOT NULL,
	episode_numbers INTEGER[] NOT NULL,
	evidence JSONB NOT NULL,
	asset_id UUID,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_prod_bible_entity_bible_scope FOREIGN KEY(bible_id, project_id, workspace_id) REFERENCES scr_production_bibles (id, project_id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_scr_prod_bible_entity_asset_workspace FOREIGN KEY(asset_id, workspace_id) REFERENCES ast_assets (id, workspace_id),
	CONSTRAINT ck_scr_prod_bible_entity_kind CHECK (kind IN ('character', 'location', 'prop', 'costume', 'visual_style', 'voice')),
	CONSTRAINT ck_scr_prod_bible_entity_stable_spec CHECK (jsonb_typeof(stable_spec) = 'object'),
	CONSTRAINT ck_scr_prod_bible_entity_evidence CHECK (jsonb_typeof(evidence) = 'array'),
	CONSTRAINT uq_scr_prod_bible_entity_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_prod_bible_entity_scope UNIQUE (id, bible_id, project_id, workspace_id),
	CONSTRAINT uq_scr_prod_bible_entity_key UNIQUE (bible_id, entity_key),
	CONSTRAINT uq_scr_prod_bible_entity_asset UNIQUE (bible_id, asset_id)
);

CREATE INDEX ix_scr_prod_bible_entity_asset ON scr_production_bible_entities (asset_id);

CREATE INDEX ix_scr_prod_bible_entity_name ON scr_production_bible_entities (bible_id, kind, normalized_name);

CREATE TABLE scr_production_bible_world_entries (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	bible_id UUID NOT NULL,
	entry_key VARCHAR(100) NOT NULL,
	category VARCHAR(80) NOT NULL,
	title VARCHAR(200) NOT NULL,
	facts TEXT[] NOT NULL,
	rules TEXT[] NOT NULL,
	entity_keys VARCHAR(100)[] NOT NULL,
	episode_numbers INTEGER[] NOT NULL,
	evidence JSONB NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_prod_bible_world_bible_scope FOREIGN KEY(bible_id, project_id, workspace_id) REFERENCES scr_production_bibles (id, project_id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT ck_scr_prod_bible_world_category CHECK (char_length(trim(category)) >= 1),
	CONSTRAINT ck_scr_prod_bible_world_evidence CHECK (jsonb_typeof(evidence) = 'array'),
	CONSTRAINT ck_scr_prod_bible_world_content CHECK (cardinality(facts) >= 1 OR cardinality(rules) >= 1),
	CONSTRAINT uq_scr_prod_bible_world_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_prod_bible_world_key UNIQUE (bible_id, entry_key)
);

CREATE INDEX ix_scr_prod_bible_world_category ON scr_production_bible_world_entries (bible_id, category);

CREATE TABLE sbd_draft_batches (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	input_script_version_id UUID NOT NULL,
	narrative_structure_id UUID NOT NULL,
	narrative_revision INTEGER NOT NULL,
	narrative_dependency_hash VARCHAR(64) NOT NULL,
	production_bible_id UUID,
	production_bible_revision INTEGER,
	production_bible_result_hash VARCHAR(64),
	production_bible_world JSONB NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	target_duration_ms INTEGER NOT NULL,
	aspect_ratio VARCHAR(10) NOT NULL,
	visual_style VARCHAR(200),
	engine_version VARCHAR(80) NOT NULL,
	model_name VARCHAR(160) NOT NULL,
	prompt_version VARCHAR(80) NOT NULL,
	schema_version VARCHAR(80) NOT NULL,
	base_order_hash VARCHAR(64) NOT NULL,
	base_shot_hash VARCHAR(64) NOT NULL,
	base_shots JSONB NOT NULL,
	agent_checkpoint JSONB,
	agent_checkpoint_revision INTEGER NOT NULL,
	agent_checkpoint_updated_at TIMESTAMP WITH TIME ZONE,
	agent_run_token UUID,
	agent_lease_expires_at TIMESTAMP WITH TIME ZONE,
	task_id UUID,
	status VARCHAR(30) NOT NULL,
	provider_result_hash VARCHAR(64),
	error_code VARCHAR(80),
	approve_idempotency_key VARCHAR(200),
	approve_command_hash VARCHAR(64),
	apply_idempotency_key VARCHAR(200),
	apply_command_hash VARCHAR(64),
	apply_result JSONB NOT NULL,
	revision INTEGER NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_draft_batch_project FOREIGN KEY(project_id, workspace_id) REFERENCES prj_projects (id, workspace_id),
	CONSTRAINT fk_sbd_draft_batch_episode FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT fk_sbd_draft_batch_script FOREIGN KEY(input_script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT fk_sbd_draft_batch_narrative FOREIGN KEY(narrative_structure_id, input_script_version_id, episode_id, workspace_id) REFERENCES scr_narrative_structures (id, script_version_id, episode_id, workspace_id),
	CONSTRAINT fk_sbd_draft_batch_task FOREIGN KEY(task_id, workspace_id) REFERENCES prod_tasks (id, workspace_id),
	CONSTRAINT fk_sbd_draft_batch_production_bible FOREIGN KEY(production_bible_id, workspace_id) REFERENCES scr_production_bibles (id, workspace_id),
	CONSTRAINT ck_sbd_draft_batch_status CHECK (status IN ('queued', 'running', 'needs_review', 'approved', 'applied', 'failed', 'unknown', 'cancelled')),
	CONSTRAINT ck_sbd_draft_narrative_revision CHECK (narrative_revision >= 1),
	CONSTRAINT ck_sbd_draft_batch_revision CHECK (revision >= 1),
	CONSTRAINT ck_sbd_draft_batch_production_bible_snapshot CHECK ((production_bible_id IS NULL AND production_bible_revision IS NULL AND production_bible_result_hash IS NULL) OR (production_bible_id IS NOT NULL AND production_bible_revision >= 1 AND char_length(production_bible_result_hash) = 64)),
	CONSTRAINT ck_sbd_draft_batch_agent_checkpoint CHECK ((agent_checkpoint IS NULL AND agent_checkpoint_revision = 0 AND agent_checkpoint_updated_at IS NULL) OR (agent_checkpoint IS NOT NULL AND agent_checkpoint_revision >= 1 AND agent_checkpoint_updated_at IS NOT NULL)),
	CONSTRAINT ck_sbd_draft_batch_agent_lease CHECK ((agent_run_token IS NULL AND agent_lease_expires_at IS NULL) OR (agent_run_token IS NOT NULL AND agent_lease_expires_at IS NOT NULL)),
	CONSTRAINT ck_sbd_draft_batch_duration CHECK (target_duration_ms >= 1000 AND target_duration_ms <= 7200000),
	CONSTRAINT uq_sbd_draft_batch_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_sbd_draft_batch_scope UNIQUE (id, episode_id, workspace_id),
	CONSTRAINT uq_sbd_draft_batch_task UNIQUE (task_id),
	CONSTRAINT uq_sbd_draft_batch_idempotency UNIQUE (episode_id, idempotency_key),
	CONSTRAINT uq_sbd_draft_approve_idempotency UNIQUE (workspace_id, approve_idempotency_key),
	CONSTRAINT uq_sbd_draft_apply_idempotency UNIQUE (workspace_id, apply_idempotency_key),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_sbd_draft_episode_created ON sbd_draft_batches (episode_id, created_at);

CREATE INDEX ix_sbd_draft_workspace_status ON sbd_draft_batches (workspace_id, status);

CREATE TABLE scr_episode_segment_origins (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	import_commit_id UUID NOT NULL,
	proposal_id UUID NOT NULL,
	document_revision_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	source_id UUID NOT NULL,
	draft_version_id UUID NOT NULL,
	published_version_id UUID,
	position INTEGER NOT NULL,
	source_start INTEGER NOT NULL,
	source_end INTEGER NOT NULL,
	source_hash VARCHAR(64) NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_segment_origin_commit_workspace FOREIGN KEY(import_commit_id, workspace_id) REFERENCES scr_import_commits (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_scr_segment_origin_proposal_workspace FOREIGN KEY(proposal_id, workspace_id) REFERENCES scr_episode_proposals (id, workspace_id),
	CONSTRAINT fk_scr_segment_origin_revision_workspace FOREIGN KEY(document_revision_id, workspace_id) REFERENCES scr_document_revisions (id, workspace_id),
	CONSTRAINT fk_scr_segment_origin_episode_workspace FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT fk_scr_segment_origin_source_workspace FOREIGN KEY(source_id, workspace_id) REFERENCES scr_script_sources (id, workspace_id),
	CONSTRAINT fk_scr_segment_origin_draft_workspace FOREIGN KEY(draft_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT fk_scr_segment_origin_published_workspace FOREIGN KEY(published_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT ck_scr_segment_origin_position CHECK (position >= 1),
	CONSTRAINT ck_scr_segment_origin_source_start CHECK (source_start >= 0),
	CONSTRAINT ck_scr_segment_origin_source_range CHECK (source_end > source_start),
	CONSTRAINT uq_scr_segment_origin_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_segment_origin_commit_position UNIQUE (import_commit_id, position),
	CONSTRAINT uq_scr_segment_origin_commit_episode UNIQUE (import_commit_id, episode_id)
);

CREATE INDEX ix_scr_segment_origin_revision_range ON scr_episode_segment_origins (document_revision_id, source_start);

CREATE TABLE scr_extraction_candidates (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	batch_id UUID NOT NULL,
	candidate_key VARCHAR(100) NOT NULL,
	kind VARCHAR(30) NOT NULL,
	source_start INTEGER NOT NULL,
	source_end INTEGER NOT NULL,
	proposal JSONB NOT NULL,
	confidence_note TEXT,
	required BOOLEAN NOT NULL,
	status VARCHAR(30) NOT NULL,
	revision INTEGER NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_candidate_batch_workspace FOREIGN KEY(batch_id, workspace_id) REFERENCES scr_extraction_batches (id, workspace_id),
	CONSTRAINT ck_scr_candidate_kind CHECK (kind IN ('scene', 'dialogue', 'asset', 'asset_occurrence', 'shot', 'continuity')),
	CONSTRAINT ck_scr_candidate_status CHECK (status IN ('pending', 'accepted', 'linked', 'merged', 'ignored')),
	CONSTRAINT ck_scr_candidate_source_start CHECK (source_start >= 0),
	CONSTRAINT ck_scr_candidate_source_range CHECK (source_end > source_start),
	CONSTRAINT ck_scr_candidate_revision CHECK (revision >= 1),
	CONSTRAINT uq_scr_candidate_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_candidate_batch_key UNIQUE (batch_id, candidate_key)
);

CREATE INDEX ix_scr_candidate_batch_status_range ON scr_extraction_candidates (batch_id, status, source_start, source_end);

CREATE TABLE scr_narrative_unit_versions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	structure_id UUID NOT NULL,
	script_version_id UUID NOT NULL,
	unit_id UUID NOT NULL,
	version_no INTEGER NOT NULL,
	structure_revision INTEGER NOT NULL,
	position INTEGER NOT NULL,
	source_start INTEGER NOT NULL,
	source_end INTEGER NOT NULL,
	exact_text TEXT NOT NULL,
	text_hash VARCHAR(64) NOT NULL,
	prefix_text VARCHAR(120) NOT NULL,
	suffix_text VARCHAR(120) NOT NULL,
	required_for_coverage BOOLEAN NOT NULL,
	payload JSONB NOT NULL,
	source_scene_id UUID,
	source_dialogue_id UUID,
	origin VARCHAR(30) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_narrative_version_structure_scope FOREIGN KEY(structure_id, script_version_id, episode_id, workspace_id) REFERENCES scr_narrative_structures (id, script_version_id, episode_id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_scr_narrative_version_unit_scope FOREIGN KEY(unit_id, episode_id, workspace_id) REFERENCES scr_narrative_units (id, episode_id, workspace_id),
	CONSTRAINT fk_scr_narrative_version_script_workspace FOREIGN KEY(script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT fk_scr_narrative_version_scene_workspace FOREIGN KEY(source_scene_id, workspace_id) REFERENCES scr_scenes (id, workspace_id),
	CONSTRAINT fk_scr_narrative_version_dialogue_workspace FOREIGN KEY(source_dialogue_id, workspace_id) REFERENCES scr_dialogues (id, workspace_id),
	CONSTRAINT ck_scr_narrative_version_number CHECK (version_no >= 1),
	CONSTRAINT ck_scr_narrative_version_structure_revision CHECK (structure_revision >= 1),
	CONSTRAINT ck_scr_narrative_version_position CHECK (position >= 1),
	CONSTRAINT ck_scr_narrative_version_start CHECK (source_start >= 0),
	CONSTRAINT ck_scr_narrative_version_range CHECK (source_end > source_start),
	CONSTRAINT ck_scr_narrative_version_origin CHECK (origin IN ('deterministic', 'manual')),
	CONSTRAINT uq_scr_narrative_version_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_narrative_version_unit_scope UNIQUE (id, unit_id, episode_id, workspace_id),
	CONSTRAINT uq_scr_narrative_version_number UNIQUE (unit_id, version_no),
	CONSTRAINT uq_scr_narrative_version_structure_position UNIQUE (structure_id, structure_revision, position),
	CONSTRAINT uq_scr_narrative_version_structure_unit UNIQUE (structure_id, structure_revision, unit_id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_narrative_version_unit_created ON scr_narrative_unit_versions (unit_id, created_at);

CREATE INDEX ix_scr_narrative_version_script_range ON scr_narrative_unit_versions (script_version_id, source_start);

CREATE TABLE scr_production_bible_entity_states (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	bible_id UUID NOT NULL,
	entity_id UUID NOT NULL,
	state_key VARCHAR(80) NOT NULL,
	label VARCHAR(120) NOT NULL,
	state_spec JSONB NOT NULL,
	episode_numbers INTEGER[] NOT NULL,
	evidence JSONB NOT NULL,
	asset_state_id UUID,
	asset_version_id UUID,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_prod_bible_state_entity_scope FOREIGN KEY(entity_id, bible_id, project_id, workspace_id) REFERENCES scr_production_bible_entities (id, bible_id, project_id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_scr_prod_bible_state_asset_state_workspace FOREIGN KEY(asset_state_id, workspace_id) REFERENCES ast_asset_states (id, workspace_id),
	CONSTRAINT fk_scr_prod_bible_state_asset_version_workspace FOREIGN KEY(asset_version_id, workspace_id) REFERENCES ast_asset_versions (id, workspace_id),
	CONSTRAINT ck_scr_prod_bible_state_spec CHECK (jsonb_typeof(state_spec) = 'object'),
	CONSTRAINT ck_scr_prod_bible_state_evidence CHECK (jsonb_typeof(evidence) = 'array'),
	CONSTRAINT ck_scr_prod_bible_state_materialization CHECK ((asset_state_id IS NULL AND asset_version_id IS NULL) OR (asset_state_id IS NOT NULL AND asset_version_id IS NOT NULL)),
	CONSTRAINT uq_scr_prod_bible_state_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_scr_prod_bible_state_key UNIQUE (entity_id, state_key),
	CONSTRAINT uq_scr_prod_bible_state_asset_state UNIQUE (bible_id, asset_state_id),
	CONSTRAINT uq_scr_prod_bible_state_asset_version UNIQUE (bible_id, asset_version_id)
);

CREATE INDEX ix_scr_prod_bible_state_bible_entity ON scr_production_bible_entity_states (bible_id, entity_id);

CREATE TABLE sbd_draft_input_assets (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	batch_id UUID NOT NULL,
	asset_id UUID NOT NULL,
	asset_state_id UUID NOT NULL,
	asset_version_id UUID NOT NULL,
	position INTEGER NOT NULL,
	kind VARCHAR(30) NOT NULL,
	name VARCHAR(200) NOT NULL,
	state_label VARCHAR(120) NOT NULL,
	state_revision INTEGER NOT NULL,
	readiness_hash VARCHAR(64) NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_draft_asset_batch FOREIGN KEY(batch_id, workspace_id) REFERENCES sbd_draft_batches (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_sbd_draft_asset_version FOREIGN KEY(asset_version_id, asset_state_id, asset_id, workspace_id) REFERENCES ast_asset_versions (id, asset_state_id, asset_id, workspace_id),
	CONSTRAINT ck_sbd_draft_asset_position CHECK (position >= 1),
	CONSTRAINT ck_sbd_draft_asset_revision CHECK (state_revision >= 1),
	CONSTRAINT uq_sbd_draft_asset_input UNIQUE (batch_id, asset_version_id, workspace_id),
	CONSTRAINT uq_sbd_draft_asset_state UNIQUE (batch_id, asset_state_id),
	CONSTRAINT uq_sbd_draft_asset_position UNIQUE (batch_id, position)
);

CREATE TABLE sbd_draft_shots (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	batch_id UUID NOT NULL,
	proposal_key VARCHAR(120) NOT NULL,
	position INTEGER NOT NULL,
	title VARCHAR(200) NOT NULL,
	spec JSONB NOT NULL,
	content_hash VARCHAR(64) NOT NULL,
	risk_codes VARCHAR(80)[] NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_draft_shot_batch FOREIGN KEY(batch_id, workspace_id) REFERENCES sbd_draft_batches (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT ck_sbd_draft_shot_position CHECK (position >= 1 AND position <= 120),
	CONSTRAINT uq_sbd_draft_shot_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_sbd_draft_shot_scope UNIQUE (id, batch_id, workspace_id),
	CONSTRAINT uq_sbd_draft_shot_key UNIQUE (batch_id, proposal_key),
	CONSTRAINT uq_sbd_draft_shot_position UNIQUE (batch_id, position)
);

CREATE TABLE ast_asset_occurrences (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	asset_state_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	narrative_unit_id UUID NOT NULL,
	narrative_unit_version_id UUID NOT NULL,
	sequence INTEGER NOT NULL,
	decision VARCHAR(20) NOT NULL,
	origin VARCHAR(30) NOT NULL,
	evidence_hash VARCHAR(64) NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_ast_occurrence_state_workspace FOREIGN KEY(asset_state_id, workspace_id) REFERENCES ast_asset_states (id, workspace_id),
	CONSTRAINT fk_ast_occurrence_unit_scope FOREIGN KEY(narrative_unit_id, episode_id, workspace_id) REFERENCES scr_narrative_units (id, episode_id, workspace_id),
	CONSTRAINT fk_ast_occurrence_unit_version_scope FOREIGN KEY(narrative_unit_version_id, narrative_unit_id, episode_id, workspace_id) REFERENCES scr_narrative_unit_versions (id, unit_id, episode_id, workspace_id),
	CONSTRAINT ck_ast_occurrence_sequence CHECK (sequence >= 1),
	CONSTRAINT ck_ast_occurrence_decision CHECK (decision IN ('link', 'unlink')),
	CONSTRAINT ck_ast_occurrence_origin CHECK (origin IN ('manual', 'script_candidate')),
	CONSTRAINT uq_ast_occurrence_state_sequence UNIQUE (asset_state_id, sequence),
	CONSTRAINT uq_ast_occurrence_state_idempotency UNIQUE (asset_state_id, idempotency_key),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_ast_occurrence_episode_state ON ast_asset_occurrences (episode_id, asset_state_id);

CREATE INDEX ix_ast_occurrence_unit_created ON ast_asset_occurrences (narrative_unit_id, created_at);

CREATE TABLE scr_candidate_decisions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	candidate_id UUID NOT NULL,
	sequence INTEGER NOT NULL,
	decision_key VARCHAR(200) NOT NULL,
	action VARCHAR(40) NOT NULL,
	payload JSONB NOT NULL,
	downstream_type VARCHAR(40),
	downstream_id UUID,
	actor_id UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_scr_decision_candidate_workspace FOREIGN KEY(candidate_id, workspace_id) REFERENCES scr_extraction_candidates (id, workspace_id),
	CONSTRAINT ck_scr_decision_sequence CHECK (sequence >= 1),
	CONSTRAINT ck_scr_decision_action CHECK (action IN ('accept_new', 'accept_with_changes', 'link_existing', 'merge_into', 'ignore')),
	CONSTRAINT uq_scr_decision_candidate_sequence UNIQUE (candidate_id, sequence),
	CONSTRAINT uq_scr_decision_candidate_key UNIQUE (candidate_id, decision_key),
	FOREIGN KEY(actor_id) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_scr_decision_candidate_created ON scr_candidate_decisions (candidate_id, created_at);

CREATE TABLE sbd_shots (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	position INTEGER NOT NULL,
	title VARCHAR(200) NOT NULL,
	source_script_version_id UUID NOT NULL,
	source_scene_id UUID NOT NULL,
	source_candidate_id UUID,
	source_draft_shot_id UUID,
	creation_key VARCHAR(200),
	status VARCHAR(20) NOT NULL,
	current_spec_version_id UUID,
	revision INTEGER NOT NULL,
	archived_at TIMESTAMP WITH TIME ZONE,
	archived_by UUID,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_shot_episode_workspace FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT fk_sbd_shot_script_workspace FOREIGN KEY(source_script_version_id, workspace_id) REFERENCES scr_script_versions (id, workspace_id),
	CONSTRAINT fk_sbd_shot_scene_workspace FOREIGN KEY(source_scene_id, workspace_id) REFERENCES scr_scenes (id, workspace_id),
	CONSTRAINT fk_sbd_shot_candidate_workspace FOREIGN KEY(source_candidate_id, workspace_id) REFERENCES scr_extraction_candidates (id, workspace_id),
	CONSTRAINT fk_sbd_shot_draft_workspace FOREIGN KEY(source_draft_shot_id, workspace_id) REFERENCES sbd_draft_shots (id, workspace_id),
	CONSTRAINT ck_sbd_shot_position CHECK (position >= 1),
	CONSTRAINT ck_sbd_shot_status CHECK (status IN ('active', 'archived')),
	CONSTRAINT ck_sbd_shot_single_origin CHECK (source_candidate_id IS NULL OR source_draft_shot_id IS NULL),
	CONSTRAINT ck_sbd_shot_revision CHECK (revision >= 1),
	CONSTRAINT uq_sbd_shot_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_sbd_shot_episode_scope UNIQUE (id, episode_id, workspace_id),
	CONSTRAINT uq_sbd_shot_workspace_creation_key UNIQUE (workspace_id, creation_key),
	FOREIGN KEY(archived_by) REFERENCES idn_user_accounts (id),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE UNIQUE INDEX uq_sbd_shot_active_position ON sbd_shots (episode_id, position) WHERE status = 'active';

CREATE INDEX ix_sbd_shot_episode_status_position ON sbd_shots (episode_id, status, position);

CREATE INDEX ix_sbd_shot_script_scene ON sbd_shots (source_script_version_id, source_scene_id);

CREATE UNIQUE INDEX uq_sbd_shot_workspace_draft ON sbd_shots (workspace_id, source_draft_shot_id) WHERE source_draft_shot_id IS NOT NULL;

CREATE UNIQUE INDEX uq_sbd_shot_workspace_candidate ON sbd_shots (workspace_id, source_candidate_id) WHERE source_candidate_id IS NOT NULL;

CREATE TABLE sbd_draft_input_units (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	batch_id UUID NOT NULL,
	narrative_unit_id UUID NOT NULL,
	unit_version_id UUID NOT NULL,
	position INTEGER NOT NULL,
	kind VARCHAR(30) NOT NULL,
	exact_text TEXT NOT NULL,
	text_hash VARCHAR(64) NOT NULL,
	required_for_coverage BOOLEAN NOT NULL,
	source_scene_id UUID,
	source_dialogue_id UUID,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_draft_unit_batch FOREIGN KEY(batch_id, episode_id, workspace_id) REFERENCES sbd_draft_batches (id, episode_id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_sbd_draft_unit_version FOREIGN KEY(unit_version_id, narrative_unit_id, episode_id, workspace_id) REFERENCES scr_narrative_unit_versions (id, unit_id, episode_id, workspace_id),
	CONSTRAINT ck_sbd_draft_unit_position CHECK (position >= 1),
	CONSTRAINT uq_sbd_draft_unit_input UNIQUE (batch_id, unit_version_id, workspace_id),
	CONSTRAINT uq_sbd_draft_unit_position UNIQUE (batch_id, position)
);

CREATE TABLE sbd_draft_asset_refs (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	batch_id UUID NOT NULL,
	draft_shot_id UUID NOT NULL,
	slot_key VARCHAR(100) NOT NULL,
	role VARCHAR(30) NOT NULL,
	asset_version_id UUID NOT NULL,
	subject_key VARCHAR(100),
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_draft_ref_shot FOREIGN KEY(draft_shot_id, batch_id, workspace_id) REFERENCES sbd_draft_shots (id, batch_id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_sbd_draft_ref_input FOREIGN KEY(batch_id, asset_version_id, workspace_id) REFERENCES sbd_draft_input_assets (batch_id, asset_version_id, workspace_id),
	CONSTRAINT ck_sbd_draft_ref_role CHECK (role IN ('location', 'character', 'prop', 'costume', 'visual_style', 'voice')),
	CONSTRAINT uq_sbd_draft_ref_slot UNIQUE (draft_shot_id, slot_key)
);

CREATE TABLE sbd_draft_decisions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	batch_id UUID NOT NULL,
	draft_shot_id UUID NOT NULL,
	sequence INTEGER NOT NULL,
	action VARCHAR(20) NOT NULL,
	target JSONB,
	command_hash VARCHAR(64) NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	actor_id UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_draft_decision_shot FOREIGN KEY(draft_shot_id, batch_id, workspace_id) REFERENCES sbd_draft_shots (id, batch_id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT ck_sbd_draft_decision_action CHECK (action IN ('accepted', 'modified', 'ignored')),
	CONSTRAINT ck_sbd_draft_decision_sequence CHECK (sequence >= 1),
	CONSTRAINT ck_sbd_draft_decision_target CHECK ((action = 'modified') = (target IS NOT NULL)),
	CONSTRAINT uq_sbd_draft_decision_sequence UNIQUE (batch_id, sequence),
	CONSTRAINT uq_sbd_draft_decision_idempotency UNIQUE (workspace_id, idempotency_key),
	FOREIGN KEY(actor_id) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_sbd_draft_decision_shot ON sbd_draft_decisions (draft_shot_id, sequence);

CREATE TABLE sbd_shot_spec_versions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	shot_id UUID NOT NULL,
	version_no INTEGER NOT NULL,
	schema_version INTEGER NOT NULL,
	spec JSONB NOT NULL,
	content_hash VARCHAR(64) NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_spec_shot_workspace FOREIGN KEY(shot_id, workspace_id) REFERENCES sbd_shots (id, workspace_id) DEFERRABLE INITIALLY DEFERRED,
	CONSTRAINT ck_sbd_spec_version_number CHECK (version_no >= 1),
	CONSTRAINT ck_sbd_spec_schema_version CHECK (schema_version = 1),
	CONSTRAINT uq_sbd_spec_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_sbd_spec_shot_scope UNIQUE (id, shot_id, workspace_id),
	CONSTRAINT uq_sbd_spec_version_number UNIQUE (shot_id, version_no),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_sbd_spec_input_hash ON sbd_shot_spec_versions (input_hash);

CREATE TABLE sbd_draft_shot_units (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	batch_id UUID NOT NULL,
	draft_shot_id UUID NOT NULL,
	unit_version_id UUID NOT NULL,
	position INTEGER NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_draft_shot_unit_shot FOREIGN KEY(draft_shot_id, batch_id, workspace_id) REFERENCES sbd_draft_shots (id, batch_id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_sbd_draft_shot_unit_input FOREIGN KEY(batch_id, unit_version_id, workspace_id) REFERENCES sbd_draft_input_units (batch_id, unit_version_id, workspace_id),
	CONSTRAINT ck_sbd_draft_shot_unit_position CHECK (position >= 1),
	CONSTRAINT uq_sbd_draft_shot_unit UNIQUE (draft_shot_id, unit_version_id),
	CONSTRAINT uq_sbd_draft_shot_unit_position UNIQUE (draft_shot_id, position)
);

CREATE TABLE prod_generation_requests (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	project_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	shot_id UUID NOT NULL,
	shot_spec_version_id UUID NOT NULL,
	capability_id UUID NOT NULL,
	capability_config_version INTEGER NOT NULL,
	parameter_snapshot JSONB NOT NULL,
	warning_acknowledgements JSONB NOT NULL,
	shot_spec_input_hash VARCHAR(64) NOT NULL,
	input_hash VARCHAR(64) NOT NULL,
	preflight_hash VARCHAR(64) NOT NULL,
	preflight_expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
	high_cost_confirmed BOOLEAN NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	requested_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_prod_request_project_workspace FOREIGN KEY(project_id, workspace_id) REFERENCES prj_projects (id, workspace_id),
	CONSTRAINT fk_prod_request_episode_workspace FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT fk_prod_request_shot_workspace FOREIGN KEY(shot_id, workspace_id) REFERENCES sbd_shots (id, workspace_id),
	CONSTRAINT fk_prod_request_spec_workspace FOREIGN KEY(shot_spec_version_id, workspace_id) REFERENCES sbd_shot_spec_versions (id, workspace_id),
	CONSTRAINT ck_prod_request_capability_version CHECK (capability_config_version >= 1),
	CONSTRAINT uq_prod_request_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_prod_request_workspace_idempotency UNIQUE (workspace_id, idempotency_key),
	FOREIGN KEY(capability_id) REFERENCES prod_model_capabilities (id),
	FOREIGN KEY(requested_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_prod_request_shot_created ON prod_generation_requests (shot_id, created_at);

CREATE INDEX ix_prod_request_input_hash ON prod_generation_requests (input_hash);

CREATE INDEX ix_prod_request_project_created ON prod_generation_requests (project_id, created_at);

CREATE TABLE sbd_asset_references (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	shot_spec_version_id UUID NOT NULL,
	slot_key VARCHAR(100) NOT NULL,
	role VARCHAR(30) NOT NULL,
	asset_version_id UUID NOT NULL,
	asset_state_id UUID NOT NULL,
	asset_id UUID NOT NULL,
	binding_source VARCHAR(30) NOT NULL,
	subject_key VARCHAR(100),
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_asset_ref_spec_workspace FOREIGN KEY(shot_spec_version_id, workspace_id) REFERENCES sbd_shot_spec_versions (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_sbd_asset_ref_version_scope FOREIGN KEY(asset_version_id, asset_state_id, asset_id, workspace_id) REFERENCES ast_asset_versions (id, asset_state_id, asset_id, workspace_id),
	CONSTRAINT ck_sbd_asset_ref_role CHECK (role IN ('location', 'character', 'prop', 'costume', 'visual_style', 'voice')),
	CONSTRAINT ck_sbd_asset_ref_binding_source CHECK (binding_source IN ('manual', 'ai')),
	CONSTRAINT uq_sbd_asset_ref_spec_slot UNIQUE (shot_spec_version_id, slot_key)
);

CREATE INDEX ix_sbd_asset_ref_asset_version ON sbd_asset_references (asset_version_id);

CREATE INDEX ix_sbd_asset_ref_state ON sbd_asset_references (asset_state_id);

CREATE TABLE sbd_narrative_references (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	shot_id UUID NOT NULL,
	shot_spec_version_id UUID NOT NULL,
	narrative_unit_id UUID NOT NULL,
	unit_version_id UUID NOT NULL,
	channel VARCHAR(20) NOT NULL,
	role VARCHAR(30) NOT NULL,
	coverage_mode VARCHAR(20) NOT NULL,
	segment_start INTEGER,
	segment_end INTEGER,
	segment_key VARCHAR(50) NOT NULL,
	contribution VARCHAR(20) NOT NULL,
	origin VARCHAR(20) NOT NULL,
	created_by UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_narrative_ref_shot_scope FOREIGN KEY(shot_id, episode_id, workspace_id) REFERENCES sbd_shots (id, episode_id, workspace_id),
	CONSTRAINT fk_sbd_narrative_ref_spec_scope FOREIGN KEY(shot_spec_version_id, shot_id, workspace_id) REFERENCES sbd_shot_spec_versions (id, shot_id, workspace_id),
	CONSTRAINT fk_sbd_narrative_ref_unit_scope FOREIGN KEY(unit_version_id, narrative_unit_id, episode_id, workspace_id) REFERENCES scr_narrative_unit_versions (id, unit_id, episode_id, workspace_id),
	CONSTRAINT ck_sbd_narrative_ref_channel CHECK (channel IN ('visual', 'audio', 'both')),
	CONSTRAINT ck_sbd_narrative_ref_role CHECK (role IN ('primary', 'dialogue', 'reaction', 'insert', 'setup', 'payoff', 'transition', 'supporting')),
	CONSTRAINT ck_sbd_narrative_ref_mode CHECK (coverage_mode IN ('full', 'partial')),
	CONSTRAINT ck_sbd_narrative_ref_segment CHECK ((coverage_mode = 'full' AND segment_start IS NULL AND segment_end IS NULL AND segment_key = 'full') OR (coverage_mode = 'partial' AND segment_start >= 0 AND segment_end > segment_start AND segment_key <> 'full')),
	CONSTRAINT ck_sbd_narrative_ref_contribution CHECK (contribution IN ('required', 'supporting')),
	CONSTRAINT ck_sbd_narrative_ref_origin CHECK (origin IN ('ai', 'human', 'migrated')),
	CONSTRAINT uq_sbd_narrative_ref_edge UNIQUE (shot_spec_version_id, unit_version_id, channel, segment_key),
	FOREIGN KEY(created_by) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_sbd_narrative_ref_episode ON sbd_narrative_references (episode_id);

CREATE INDEX ix_sbd_narrative_ref_unit ON sbd_narrative_references (unit_version_id);

CREATE INDEX ix_sbd_narrative_ref_spec ON sbd_narrative_references (shot_spec_version_id);

CREATE TABLE sbd_coverage_decisions (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	episode_id UUID NOT NULL,
	sequence INTEGER NOT NULL,
	action VARCHAR(30) NOT NULL,
	narrative_unit_id UUID,
	unit_version_id UUID,
	shot_id UUID,
	shot_spec_version_id UUID,
	basis_hash VARCHAR(64) NOT NULL,
	reason VARCHAR(1000) NOT NULL,
	evidence TEXT,
	command_hash VARCHAR(64) NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	actor_id UUID NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_sbd_coverage_decision_episode FOREIGN KEY(episode_id, workspace_id) REFERENCES prj_episodes (id, workspace_id),
	CONSTRAINT fk_sbd_coverage_decision_unit FOREIGN KEY(unit_version_id, narrative_unit_id, episode_id, workspace_id) REFERENCES scr_narrative_unit_versions (id, unit_id, episode_id, workspace_id),
	CONSTRAINT fk_sbd_coverage_decision_shot FOREIGN KEY(shot_id, episode_id, workspace_id) REFERENCES sbd_shots (id, episode_id, workspace_id),
	CONSTRAINT fk_sbd_coverage_decision_spec FOREIGN KEY(shot_spec_version_id, shot_id, workspace_id) REFERENCES sbd_shot_spec_versions (id, shot_id, workspace_id),
	CONSTRAINT ck_sbd_coverage_decision_sequence CHECK (sequence >= 1),
	CONSTRAINT ck_sbd_coverage_decision_action CHECK (action IN ('approve_omission', 'revoke_omission', 'approve_invented', 'revoke_invented')),
	CONSTRAINT ck_sbd_coverage_decision_target CHECK (((action IN ('approve_omission', 'revoke_omission')) AND unit_version_id IS NOT NULL AND narrative_unit_id IS NOT NULL AND shot_spec_version_id IS NULL AND shot_id IS NULL) OR ((action IN ('approve_invented', 'revoke_invented')) AND shot_spec_version_id IS NOT NULL AND shot_id IS NOT NULL AND unit_version_id IS NULL AND narrative_unit_id IS NULL)),
	CONSTRAINT uq_sbd_coverage_decision_sequence UNIQUE (episode_id, sequence),
	CONSTRAINT uq_sbd_coverage_decision_idempotency UNIQUE (workspace_id, idempotency_key),
	FOREIGN KEY(actor_id) REFERENCES idn_user_accounts (id)
);

CREATE INDEX ix_sbd_coverage_decision_episode_created ON sbd_coverage_decisions (episode_id, sequence);

CREATE TABLE prod_generation_request_assets (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	request_id UUID NOT NULL,
	asset_version_id UUID NOT NULL,
	slot_key VARCHAR(100) NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_prod_request_asset_request_workspace FOREIGN KEY(request_id, workspace_id) REFERENCES prod_generation_requests (id, workspace_id) ON DELETE CASCADE,
	CONSTRAINT fk_prod_request_asset_version_workspace FOREIGN KEY(asset_version_id, workspace_id) REFERENCES ast_asset_versions (id, workspace_id),
	CONSTRAINT uq_prod_request_asset_slot UNIQUE (request_id, slot_key)
);

CREATE INDEX ix_prod_request_asset_version ON prod_generation_request_assets (asset_version_id);

CREATE TABLE prod_reservations (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	request_id UUID NOT NULL,
	currency VARCHAR(3) NOT NULL,
	estimated_amount NUMERIC(20, 6) NOT NULL,
	reserved_amount NUMERIC(20, 6) NOT NULL,
	status VARCHAR(20) NOT NULL,
	revision INTEGER NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_prod_reservation_request_workspace FOREIGN KEY(request_id, workspace_id) REFERENCES prod_generation_requests (id, workspace_id),
	CONSTRAINT ck_prod_reservation_status CHECK (status IN ('active', 'settled', 'released')),
	CONSTRAINT ck_prod_reservation_estimated CHECK (estimated_amount >= 0),
	CONSTRAINT ck_prod_reservation_reserved CHECK (reserved_amount >= 0),
	CONSTRAINT ck_prod_reservation_revision CHECK (revision >= 1),
	CONSTRAINT uq_prod_reservation_id_workspace UNIQUE (id, workspace_id),
	CONSTRAINT uq_prod_reservation_request UNIQUE (request_id)
);

CREATE INDEX ix_prod_reservation_workspace_status ON prod_reservations (workspace_id, status);

CREATE TABLE prod_cost_entries (
	id UUID NOT NULL,
	workspace_id UUID NOT NULL,
	reservation_id UUID NOT NULL,
	attempt_id UUID,
	entry_type VARCHAR(20) NOT NULL,
	amount NUMERIC(20, 6) NOT NULL,
	currency VARCHAR(3) NOT NULL,
	provider_bill_ref VARCHAR(200),
	idempotency_key VARCHAR(200) NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	PRIMARY KEY (id),
	CONSTRAINT fk_prod_cost_reservation_workspace FOREIGN KEY(reservation_id, workspace_id) REFERENCES prod_reservations (id, workspace_id),
	CONSTRAINT fk_prod_cost_attempt_workspace FOREIGN KEY(attempt_id, workspace_id) REFERENCES prod_attempts (id, workspace_id),
	CONSTRAINT ck_prod_cost_entry_type CHECK (entry_type IN ('reserve', 'settle', 'release', 'adjust')),
	CONSTRAINT uq_prod_cost_entry_idempotency UNIQUE (reservation_id, idempotency_key)
);

CREATE INDEX ix_prod_cost_reservation_created ON prod_cost_entries (reservation_id, created_at);

ALTER TABLE sbd_shots ADD CONSTRAINT fk_sbd_shot_current_spec_workspace FOREIGN KEY(current_spec_version_id, workspace_id) REFERENCES sbd_shot_spec_versions (id, workspace_id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE gov_consents ADD CONSTRAINT fk_gov_consent_current_revision_workspace FOREIGN KEY(current_revision_id, workspace_id) REFERENCES gov_consent_revisions (id, workspace_id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ast_assets ADD CONSTRAINT fk_ast_asset_current_name FOREIGN KEY(id, workspace_id, name_revision) REFERENCES ast_asset_name_revisions (asset_id, workspace_id, revision_no) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE scr_narrative_units ADD CONSTRAINT fk_scr_narrative_unit_current_workspace FOREIGN KEY(current_version_id, workspace_id) REFERENCES scr_narrative_unit_versions (id, workspace_id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE ast_asset_states ADD CONSTRAINT fk_ast_state_current_version_scope FOREIGN KEY(current_version_id, id, asset_id, workspace_id) REFERENCES ast_asset_versions (id, asset_state_id, asset_id, workspace_id) DEFERRABLE INITIALLY DEFERRED;

ALTER TABLE med_media_objects ADD CONSTRAINT fk_med_object_current_version_workspace FOREIGN KEY(current_version_id, workspace_id) REFERENCES med_media_versions (id, workspace_id) DEFERRABLE INITIALLY DEFERRED;
