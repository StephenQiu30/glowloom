-- Expand only. The caller owns the transaction and must set a bounded lock timeout.
-- This file never creates a database/role/schema, resets data, or infers identities.
-- Run once on the existing pre-email schema; rollback the transaction for rehearsal.
ALTER TABLE identity."user"
    ADD COLUMN email_key text,
    ADD COLUMN email_verified_at timestamptz,
    ADD COLUMN email_version bigint NOT NULL DEFAULT 1 CHECK (email_version > 0),
    ADD COLUMN platform_role text NOT NULL DEFAULT 'creator' CHECK (platform_role IN ('creator','platform_admin')),
    ADD COLUMN platform_epoch bigint NOT NULL DEFAULT 1 CHECK (platform_epoch > 0),
    ADD COLUMN avatar_asset_id uuid REFERENCES media.media_asset(id) ON DELETE RESTRICT,
    ADD COLUMN terms_version text,
    ADD COLUMN terms_accepted_at timestamptz,
    ADD COLUMN theme text NOT NULL DEFAULT 'system' CHECK (theme IN ('light','dark','system')),
    ADD COLUMN prompt_suffix text NOT NULL DEFAULT '' CHECK (char_length(prompt_suffix) <= 2000),
    ADD COLUMN deleted_at timestamptz,
    ADD CONSTRAINT user_email_key_check CHECK (email_key IS NULL OR (
        octet_length(email_key) BETWEEN 3 AND 254 AND email_key = lower(btrim(email_key))
        AND email_key !~ '[^\x21-\x7e]' AND email_key ~ '^[^@]+@[^@]+$')),
    ADD CONSTRAINT user_email_proof_check CHECK ((email_key IS NULL) = (email_verified_at IS NULL)),
    ADD CONSTRAINT user_terms_check CHECK ((terms_version IS NULL) = (terms_accepted_at IS NULL));

-- session_epoch is the existing durable credential epoch. Retain one source of
-- truth during expansion; the cutover will rename it after old consumers retire.
CREATE UNIQUE INDEX user_email_key_unique ON identity."user"(email_key) WHERE email_key IS NOT NULL;
CREATE INDEX user_platform_directory_idx ON identity."user"(create_time DESC,id DESC);
CREATE INDEX user_platform_status_idx ON identity."user"(status,create_time DESC,id DESC);
CREATE INDEX user_platform_role_idx ON identity."user"(platform_role,status);

ALTER TABLE identity.user_session
    ADD COLUMN device_summary text NOT NULL DEFAULT '' CHECK (char_length(device_summary) <= 256),
    ADD COLUMN reauth_at timestamptz,
    ADD COLUMN reauth_epoch bigint CHECK (reauth_epoch > 0),
    ADD COLUMN csrf_hash bytea CHECK (octet_length(csrf_hash) = 32),
    ADD CONSTRAINT user_session_reauth_check CHECK ((reauth_at IS NULL) = (reauth_epoch IS NULL));
CREATE INDEX user_session_activity_idx ON identity.user_session(account_id,last_active_at DESC,id);
CREATE INDEX user_session_live_idx ON identity.user_session(account_id,id) WHERE revoked_at IS NULL;

CREATE TABLE identity.platform_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    registration_enabled boolean NOT NULL DEFAULT false,
    team_creation_enabled boolean NOT NULL DEFAULT false,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    updated_by uuid REFERENCES identity."user"(id) ON DELETE RESTRICT,
    update_time timestamptz NOT NULL DEFAULT now()
);
INSERT INTO identity.platform_settings(singleton) VALUES(true);

CREATE TABLE workspace.team (
    id uuid PRIMARY KEY,
    name text NOT NULL CHECK (char_length(name) BETWEEN 2 AND 50),
    avatar_asset_id uuid REFERENCES media.media_asset(id) ON DELETE RESTRICT,
    created_by uuid NOT NULL REFERENCES identity."user"(id) ON DELETE RESTRICT,
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','archived','suspended')),
    resume_status text CHECK (resume_status IN ('active','archived')),
    governance_blocked boolean NOT NULL DEFAULT false,
    auth_epoch bigint NOT NULL DEFAULT 1 CHECK (auth_epoch > 0),
    invitation_enabled boolean NOT NULL DEFAULT true,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    create_time timestamptz NOT NULL DEFAULT now(),
    update_time timestamptz NOT NULL DEFAULT now(),
    CHECK ((status = 'suspended') = (resume_status IS NOT NULL))
);
CREATE INDEX team_creator_idx ON workspace.team(created_by,status);
CREATE INDEX team_status_idx ON workspace.team(status,create_time DESC,id DESC);

CREATE TABLE workspace.team_member (
    team_id uuid NOT NULL REFERENCES workspace.team(id) ON DELETE RESTRICT,
    user_id uuid NOT NULL REFERENCES identity."user"(id) ON DELETE RESTRICT,
    role text NOT NULL CHECK (role IN ('team_admin','member')),
    status text NOT NULL CHECK (status IN ('active','suspended','removed')),
    auth_epoch bigint NOT NULL DEFAULT 1 CHECK (auth_epoch > 0),
    joined_at timestamptz NOT NULL,
    joined_by uuid REFERENCES identity."user"(id) ON DELETE RESTRICT,
    source text NOT NULL CHECK (source IN ('created','invited','migrated','transferred')),
    last_team_activity_at timestamptz,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    create_time timestamptz NOT NULL DEFAULT now(),
    update_time timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY(team_id,user_id)
);
CREATE INDEX team_member_user_idx ON workspace.team_member(user_id,status,team_id);
CREATE INDEX team_member_directory_idx ON workspace.team_member(team_id,status,joined_at DESC,user_id DESC);
CREATE INDEX team_member_admin_idx ON workspace.team_member(team_id,role,status);

CREATE TABLE workspace.creation_scope (
    id uuid PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('personal','team')),
    owner_user_id uuid REFERENCES identity."user"(id) ON DELETE RESTRICT,
    team_id uuid REFERENCES workspace.team(id) ON DELETE RESTRICT,
    physical_scope_id uuid NOT NULL REFERENCES workspace.organization(id) ON DELETE RESTRICT,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    create_time timestamptz NOT NULL DEFAULT now(),
    update_time timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'personal' AND owner_user_id IS NOT NULL AND team_id IS NULL)
        OR (kind = 'team' AND owner_user_id IS NULL AND team_id IS NOT NULL))
);
CREATE UNIQUE INDEX creation_scope_personal_unique ON workspace.creation_scope(owner_user_id) WHERE owner_user_id IS NOT NULL;
CREATE UNIQUE INDEX creation_scope_team_unique ON workspace.creation_scope(team_id) WHERE team_id IS NOT NULL;

CREATE TABLE identity.auth_challenge (
    id uuid PRIMARY KEY,
    operation_id uuid NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('registration_code','registration_grant','password_reset','email_old','email_new','deletion_confirm','deletion_cancel','migration_bind')),
    email_key text NOT NULL,
    user_id uuid REFERENCES identity."user"(id) ON DELETE RESTRICT,
    email_version bigint CHECK (email_version > 0),
    credential_epoch bigint CHECK (credential_epoch > 0),
    proof_digest bytea NOT NULL CHECK (octet_length(proof_digest) = 32),
    proof_key_id text NOT NULL,
    flow_secret_digest bytea CHECK (octet_length(flow_secret_digest) = 32),
    attempts smallint NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 5),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    invitation_id uuid,
    CHECK (expires_at > created_at)
);
CREATE INDEX auth_challenge_operation_idx ON identity.auth_challenge(operation_id,purpose);
CREATE INDEX auth_challenge_email_idx ON identity.auth_challenge(email_key,purpose,created_at DESC);
CREATE INDEX auth_challenge_expiry_idx ON identity.auth_challenge(expires_at);

CREATE TABLE identity.email_budget (
    bucket_key text PRIMARY KEY,
    sent_at timestamptz[] NOT NULL DEFAULT '{}',
    failed_at timestamptz[] NOT NULL DEFAULT '{}',
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0)
);

CREATE TABLE identity.security_event (
    id uuid PRIMARY KEY,
    actor_id uuid REFERENCES identity."user"(id) ON DELETE RESTRICT,
    target_user_id uuid REFERENCES identity."user"(id) ON DELETE RESTRICT,
    team_id uuid REFERENCES workspace.team(id) ON DELETE RESTRICT,
    scope_kind text NOT NULL,
    action text NOT NULL,
    result text NOT NULL,
    reason text NOT NULL DEFAULT '',
    request_id text NOT NULL DEFAULT '',
    safe_diff jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(safe_diff) = 'object'),
    created_at timestamptz NOT NULL
);
CREATE INDEX security_event_user_idx ON identity.security_event(target_user_id,created_at DESC,id);
CREATE INDEX security_event_team_idx ON identity.security_event(team_id,created_at DESC,id);
CREATE INDEX security_event_time_idx ON identity.security_event(created_at DESC,id);

CREATE TABLE identity.mail_delivery (
    event_id uuid PRIMARY KEY,
    challenge_id uuid REFERENCES identity.auth_challenge(id) ON DELETE RESTRICT,
    template text NOT NULL,
    recipient text NOT NULL,
    encrypted_payload bytea,
    key_id text,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','leased','submitted','delivered','bounced','failed','expired')),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    lease_until timestamptz,
    provider_message_id text,
    error_category text,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    CHECK ((encrypted_payload IS NULL) = (key_id IS NULL))
);
CREATE INDEX mail_delivery_pending_idx ON identity.mail_delivery(status,next_attempt_at,event_id);

GRANT SELECT,INSERT,UPDATE ON identity.platform_settings,identity.auth_challenge,identity.email_budget,identity.mail_delivery,
    workspace.team,workspace.team_member,workspace.creation_scope TO lanverse_app;
GRANT SELECT,INSERT ON identity.security_event TO lanverse_app;
