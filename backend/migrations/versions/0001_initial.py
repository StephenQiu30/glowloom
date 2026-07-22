"""Create the identity and catalog MVP schema."""

from typing import Any

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql


revision = "platform_0001"
down_revision = None
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.execute('CREATE SCHEMA "identity"')
    op.execute('CREATE SCHEMA "catalog"')
    _create_identity_tables()
    _create_catalog_tables()


def downgrade() -> None:
    op.drop_table("generation_examples", schema="catalog")
    op.drop_index("ix_catalog_prompt_templates_category_id", schema="catalog")
    op.drop_table("prompt_templates", schema="catalog")
    op.drop_table("categories", schema="catalog")
    op.drop_index("ix_identity_sessions_user_id", schema="identity")
    op.drop_table("sessions", schema="identity")
    op.drop_table("invitations", schema="identity")
    op.drop_table("users", schema="identity")
    op.execute('DROP SCHEMA "catalog"')
    op.execute('DROP SCHEMA "identity"')


def _uuid(name: str, **kwargs: Any) -> sa.Column[Any]:
    return sa.Column(name, postgresql.UUID(as_uuid=True), **kwargs)


def _created_at() -> sa.Column[Any]:
    return sa.Column(
        "created_at",
        sa.DateTime(timezone=True),
        nullable=False,
        server_default=sa.func.now(),
    )


def _check(expression: str, name: str) -> sa.CheckConstraint:
    return sa.CheckConstraint(expression, name=name)


def _fk(
    column: str, target: str, key: str, *, cascade: bool = False
) -> sa.ForeignKeyConstraint:
    ondelete = "CASCADE" if cascade else "RESTRICT"
    return sa.ForeignKeyConstraint(
        [column], [target], name=f"fk_{key}", ondelete=ondelete
    )


def _create_identity_tables() -> None:
    op.create_table(
        "users",
        _uuid("id", primary_key=True),
        sa.Column("email", sa.String(320), nullable=False),
        sa.Column("password_hash", sa.String(255), nullable=False),
        sa.Column("role", sa.String(16), nullable=False),
        sa.Column("is_active", sa.Boolean(), nullable=False, server_default=sa.true()),
        _created_at(),
        _check("email = lower(btrim(email))", "ck_identity_users_email_normalized"),
        _check("role IN ('creator', 'admin')", "ck_identity_users_role"),
        sa.UniqueConstraint("email", name="uq_identity_users_email"),
        schema="identity",
    )
    op.create_table(
        "invitations",
        _uuid("id", primary_key=True),
        sa.Column("email", sa.String(320), nullable=False),
        sa.Column("token_hash", sa.String(64), nullable=False),
        sa.Column("role", sa.String(16), nullable=False),
        _uuid("invited_by", nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("accepted_at", sa.DateTime(timezone=True)),
        sa.Column("revoked_at", sa.DateTime(timezone=True)),
        _created_at(),
        _check(
            "email = lower(btrim(email))", "ck_identity_invitations_email_normalized"
        ),
        _check("role IN ('creator', 'admin')", "ck_identity_invitations_role"),
        _check("expires_at > created_at", "ck_identity_invitations_expiry"),
        _check(
            "accepted_at IS NULL OR revoked_at IS NULL",
            "ck_identity_invitations_single_resolution",
        ),
        _fk(
            "invited_by",
            "identity.users.id",
            "identity_invitations_invited_by_users",
        ),
        sa.UniqueConstraint("token_hash", name="uq_identity_invitations_token_hash"),
        schema="identity",
    )
    op.create_table(
        "sessions",
        _uuid("id", primary_key=True),
        _uuid("user_id", nullable=False),
        sa.Column("token_hash", sa.String(64), nullable=False),
        sa.Column("csrf_token_hash", sa.String(64), nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("revoked_at", sa.DateTime(timezone=True)),
        _created_at(),
        _check("expires_at > created_at", "ck_identity_sessions_expiry"),
        _fk(
            "user_id",
            "identity.users.id",
            "identity_sessions_user_id_users",
            cascade=True,
        ),
        sa.UniqueConstraint("token_hash", name="uq_identity_sessions_token_hash"),
        schema="identity",
    )
    op.create_index(
        "ix_identity_sessions_user_id", "sessions", ["user_id"], schema="identity"
    )


def _create_catalog_tables() -> None:
    slug_check = "slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'"
    op.create_table(
        "categories",
        _uuid("id", primary_key=True),
        sa.Column("slug", sa.String(128), nullable=False),
        sa.Column("name", sa.String(128), nullable=False),
        _created_at(),
        _check(slug_check, "ck_catalog_categories_slug"),
        _check("btrim(name) <> ''", "ck_catalog_categories_name"),
        sa.UniqueConstraint("slug", name="uq_catalog_categories_slug"),
        schema="catalog",
    )
    op.create_table(
        "prompt_templates",
        _uuid("id", primary_key=True),
        sa.Column("slug", sa.String(128), nullable=False),
        sa.Column("title", sa.String(200), nullable=False),
        sa.Column("prompt", sa.Text(), nullable=False),
        sa.Column("negative_prompt", sa.Text()),
        sa.Column("source_model", sa.String(128), nullable=False),
        sa.Column("aspect_ratio", sa.String(32), nullable=False),
        _uuid("category_id", nullable=False),
        sa.Column("source_name", sa.String(128), nullable=False),
        sa.Column("source_url", sa.Text(), nullable=False),
        sa.Column("source_object_id", sa.String(255), nullable=False),
        sa.Column("source_revision", sa.String(128), nullable=False),
        sa.Column("source_license", sa.String(128), nullable=False),
        sa.Column("collected_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("content_hash", sa.String(64), nullable=False),
        sa.Column("status", sa.String(16), nullable=False),
        sa.Column("published_at", sa.DateTime(timezone=True), nullable=False),
        _created_at(),
        _check(slug_check, "ck_catalog_prompt_templates_slug"),
        _check(
            "btrim(title) <> '' AND btrim(prompt) <> ''",
            "ck_catalog_prompt_templates_content",
        ),
        _check(
            "content_hash ~ '^[0-9a-f]{64}$'",
            "ck_catalog_prompt_templates_content_hash",
        ),
        _check(
            "status IN ('published', 'suppressed', 'deleted')",
            "ck_catalog_prompt_templates_status",
        ),
        _fk(
            "category_id",
            "catalog.categories.id",
            "catalog_prompt_templates_category_id_categories",
        ),
        sa.UniqueConstraint("slug", name="uq_catalog_prompt_templates_slug"),
        sa.UniqueConstraint(
            "content_hash", name="uq_catalog_prompt_templates_content_hash"
        ),
        schema="catalog",
    )
    op.create_index(
        "ix_catalog_prompt_templates_category_id",
        "prompt_templates",
        ["category_id"],
        schema="catalog",
    )
    op.create_table(
        "generation_examples",
        _uuid("id", primary_key=True),
        _uuid("template_id", nullable=False),
        _uuid("asset_id", nullable=False),
        sa.Column("alt_text", sa.String(255), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        _created_at(),
        _check("position >= 0", "ck_catalog_generation_examples_position"),
        _fk(
            "template_id",
            "catalog.prompt_templates.id",
            "catalog_generation_examples_template_id_prompt_templates",
            cascade=True,
        ),
        sa.UniqueConstraint(
            "template_id",
            "asset_id",
            name="uq_catalog_generation_examples_template_asset",
        ),
        schema="catalog",
    )
