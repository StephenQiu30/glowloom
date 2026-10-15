-- Reviewed incremental expansion only. Application is an operator action; no runner applies this file.
CREATE TABLE IF NOT EXISTS identity.authority_proposal (
 id uuid PRIMARY KEY,
 kind text NOT NULL CHECK (kind IN ('platform_grant','team_transfer','platform_takeover')),
 team_id uuid REFERENCES workspace.team(id) ON DELETE RESTRICT,
 source_user_id uuid NOT NULL REFERENCES identity."user"(id) ON DELETE RESTRICT,
 target_user_id uuid NOT NULL REFERENCES identity."user"(id) ON DELETE RESTRICT,
 status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','accepted','declined','cancelled','expired')),
 expires_at timestamptz NOT NULL,
 reason text NOT NULL CHECK (char_length(reason) BETWEEN 5 AND 500),
 source_authority_revision bigint NOT NULL CHECK (source_authority_revision>0),
 target_revision bigint NOT NULL CHECK (target_revision>0),
 team_revision bigint,
 revision bigint NOT NULL DEFAULT 1 CHECK (revision>0),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 CHECK ((kind='platform_grant')=(team_id IS NULL)),
 CHECK ((team_id IS NULL)=(team_revision IS NULL)),
 CHECK (source_user_id<>target_user_id)
);
CREATE INDEX IF NOT EXISTS authority_proposal_target_idx ON identity.authority_proposal(target_user_id,status,expires_at);
CREATE INDEX IF NOT EXISTS authority_proposal_team_idx ON identity.authority_proposal(team_id,status);
CREATE UNIQUE INDEX IF NOT EXISTS authority_proposal_platform_pending_idx ON identity.authority_proposal(target_user_id) WHERE kind='platform_grant' AND status='pending';
CREATE UNIQUE INDEX IF NOT EXISTS authority_proposal_team_pending_idx ON identity.authority_proposal(team_id) WHERE kind IN ('team_transfer','platform_takeover') AND status='pending';
GRANT SELECT,INSERT,UPDATE ON identity.authority_proposal TO lanverse_app;
CREATE TABLE IF NOT EXISTS identity.invitation (
 id uuid PRIMARY KEY,
 kind text NOT NULL CHECK (kind IN ('account','team')),
 team_id uuid REFERENCES workspace.team(id) ON DELETE RESTRICT,
 email_key text NOT NULL,
 team_role text CHECK (team_role IN ('member','team_admin')),
 inviter_id uuid NOT NULL REFERENCES identity."user"(id) ON DELETE RESTRICT,
 status text NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','accepted','declined','revoked','expired')),
 token_hash bytea NOT NULL UNIQUE CHECK(octet_length(token_hash)=32),
 expires_at timestamptz NOT NULL,
 accepted_user_id uuid REFERENCES identity."user"(id) ON DELETE RESTRICT,
 membership_revision bigint,
 revision bigint NOT NULL DEFAULT 1 CHECK(revision>0),
 created_at timestamptz NOT NULL,
 updated_at timestamptz NOT NULL,
 CHECK ((kind='account' AND team_id IS NULL AND team_role IS NULL) OR (kind='team' AND team_id IS NOT NULL AND team_role IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS invitation_pending_team_idx ON identity.invitation(team_id,email_key) WHERE kind='team' AND status='pending';
CREATE UNIQUE INDEX IF NOT EXISTS invitation_pending_account_idx ON identity.invitation(email_key) WHERE kind='account' AND status='pending';
GRANT SELECT,INSERT,UPDATE ON identity.invitation TO lanverse_app;

-- Reject unsupported historical cancellation proofs explicitly; do not delete records.
ALTER TABLE identity.auth_challenge DROP CONSTRAINT auth_challenge_purpose_check;
ALTER TABLE identity.auth_challenge ADD CONSTRAINT auth_challenge_purpose_check CHECK (purpose IN ('registration_code','registration_grant','password_reset','email_old','email_new','deletion_confirm','migration_bind'));

-- Bind registration proofs and outbox attempts to the exact invitation revision.
ALTER TABLE identity.auth_challenge ADD COLUMN IF NOT EXISTS invitation_revision bigint CHECK(invitation_revision>0);
ALTER TABLE identity.mail_delivery ADD COLUMN IF NOT EXISTS invitation_id uuid REFERENCES identity.invitation(id) ON DELETE RESTRICT;
ALTER TABLE identity.mail_delivery ADD COLUMN IF NOT EXISTS invitation_revision bigint CHECK(invitation_revision>0);
ALTER TABLE identity.authority_proposal ADD COLUMN IF NOT EXISTS target_email_version bigint NOT NULL DEFAULT 1 CHECK(target_email_version>0);
ALTER TABLE identity.authority_proposal ADD COLUMN IF NOT EXISTS target_credential_epoch bigint NOT NULL DEFAULT 1 CHECK(target_credential_epoch>0);

ALTER TABLE identity.invitation ADD COLUMN IF NOT EXISTS platform_issued boolean NOT NULL DEFAULT false;
