"""Create identity fact tables."""

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql


revision = "identity_0002"
down_revision = "identity_0001"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "users",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True),
        sa.Column("email", sa.String(320), nullable=False),
        sa.Column("password_hash", sa.String(255), nullable=False),
        sa.Column("role", sa.String(16), nullable=False),
        sa.Column(
            "is_active",
            sa.Boolean(),
            nullable=False,
            server_default=sa.true(),
        ),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.func.now(),
        ),
        sa.CheckConstraint(
            "email = lower(btrim(email))",
            name="ck_identity_users_email_normalized",
        ),
        sa.CheckConstraint(
            "role IN ('creator', 'admin')",
            name="ck_identity_users_role",
        ),
        sa.UniqueConstraint("email", name="uq_identity_users_email"),
        schema="identity",
    )
    op.create_table(
        "invitations",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True),
        sa.Column("email", sa.String(320), nullable=False),
        sa.Column("token_hash", sa.String(64), nullable=False),
        sa.Column("role", sa.String(16), nullable=False),
        sa.Column("invited_by", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("accepted_at", sa.DateTime(timezone=True)),
        sa.Column("revoked_at", sa.DateTime(timezone=True)),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.func.now(),
        ),
        sa.CheckConstraint(
            "email = lower(btrim(email))",
            name="ck_identity_invitations_email_normalized",
        ),
        sa.CheckConstraint(
            "role IN ('creator', 'admin')",
            name="ck_identity_invitations_role",
        ),
        sa.CheckConstraint(
            "expires_at > created_at",
            name="ck_identity_invitations_expiry",
        ),
        sa.CheckConstraint(
            "accepted_at IS NULL OR revoked_at IS NULL",
            name="ck_identity_invitations_single_resolution",
        ),
        sa.ForeignKeyConstraint(
            ["invited_by"],
            ["identity.users.id"],
            name="fk_identity_invitations_invited_by_users",
            ondelete="RESTRICT",
        ),
        sa.UniqueConstraint(
            "token_hash",
            name="uq_identity_invitations_token_hash",
        ),
        schema="identity",
    )
    op.create_table(
        "sessions",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True),
        sa.Column("user_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("token_hash", sa.String(64), nullable=False),
        sa.Column("csrf_token_hash", sa.String(64), nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("revoked_at", sa.DateTime(timezone=True)),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.func.now(),
        ),
        sa.CheckConstraint(
            "expires_at > created_at",
            name="ck_identity_sessions_expiry",
        ),
        sa.ForeignKeyConstraint(
            ["user_id"],
            ["identity.users.id"],
            name="fk_identity_sessions_user_id_users",
            ondelete="CASCADE",
        ),
        sa.UniqueConstraint("token_hash", name="uq_identity_sessions_token_hash"),
        schema="identity",
    )
    op.create_index(
        "ix_identity_sessions_user_id",
        "sessions",
        ["user_id"],
        schema="identity",
    )


def downgrade() -> None:
    op.drop_index(
        "ix_identity_sessions_user_id",
        table_name="sessions",
        schema="identity",
    )
    op.drop_table("sessions", schema="identity")
    op.drop_table("invitations", schema="identity")
    op.drop_table("users", schema="identity")
