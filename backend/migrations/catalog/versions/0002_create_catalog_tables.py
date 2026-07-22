"""Create catalog fact tables."""

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql


revision = "catalog_0002"
down_revision = "catalog_0001"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.create_table(
        "categories",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True),
        sa.Column("slug", sa.String(128), nullable=False),
        sa.Column("name", sa.String(128), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.func.now(),
        ),
        sa.CheckConstraint(
            "slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'",
            name="ck_catalog_categories_slug",
        ),
        sa.CheckConstraint("btrim(name) <> ''", name="ck_catalog_categories_name"),
        sa.UniqueConstraint("slug", name="uq_catalog_categories_slug"),
        schema="catalog",
    )
    op.create_table(
        "prompt_templates",
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True),
        sa.Column("slug", sa.String(128), nullable=False),
        sa.Column("title", sa.String(200), nullable=False),
        sa.Column("prompt", sa.Text(), nullable=False),
        sa.Column("negative_prompt", sa.Text()),
        sa.Column("source_model", sa.String(128), nullable=False),
        sa.Column("aspect_ratio", sa.String(32), nullable=False),
        sa.Column("category_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("source_name", sa.String(128), nullable=False),
        sa.Column("source_url", sa.Text(), nullable=False),
        sa.Column("source_object_id", sa.String(255), nullable=False),
        sa.Column("source_revision", sa.String(128), nullable=False),
        sa.Column("source_license", sa.String(128), nullable=False),
        sa.Column("collected_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("content_hash", sa.String(64), nullable=False),
        sa.Column("status", sa.String(16), nullable=False),
        sa.Column("published_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.func.now(),
        ),
        sa.CheckConstraint(
            "slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'",
            name="ck_catalog_prompt_templates_slug",
        ),
        sa.CheckConstraint(
            "btrim(title) <> '' AND btrim(prompt) <> ''",
            name="ck_catalog_prompt_templates_content",
        ),
        sa.CheckConstraint(
            "content_hash ~ '^[0-9a-f]{64}$'",
            name="ck_catalog_prompt_templates_content_hash",
        ),
        sa.CheckConstraint(
            "status IN ('published', 'suppressed', 'deleted')",
            name="ck_catalog_prompt_templates_status",
        ),
        sa.ForeignKeyConstraint(
            ["category_id"],
            ["catalog.categories.id"],
            name="fk_catalog_prompt_templates_category_id_categories",
            ondelete="RESTRICT",
        ),
        sa.UniqueConstraint("slug", name="uq_catalog_prompt_templates_slug"),
        sa.UniqueConstraint(
            "content_hash",
            name="uq_catalog_prompt_templates_content_hash",
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
        sa.Column("id", postgresql.UUID(as_uuid=True), primary_key=True),
        sa.Column("template_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("asset_id", postgresql.UUID(as_uuid=True), nullable=False),
        sa.Column("alt_text", sa.String(255), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.func.now(),
        ),
        sa.CheckConstraint(
            "position >= 0",
            name="ck_catalog_generation_examples_position",
        ),
        sa.ForeignKeyConstraint(
            ["template_id"],
            ["catalog.prompt_templates.id"],
            name="fk_catalog_generation_examples_template_id_prompt_templates",
            ondelete="CASCADE",
        ),
        sa.UniqueConstraint(
            "template_id",
            "asset_id",
            name="uq_catalog_generation_examples_template_asset",
        ),
        schema="catalog",
    )


def downgrade() -> None:
    op.drop_table("generation_examples", schema="catalog")
    op.drop_index(
        "ix_catalog_prompt_templates_category_id",
        table_name="prompt_templates",
        schema="catalog",
    )
    op.drop_table("prompt_templates", schema="catalog")
    op.drop_table("categories", schema="catalog")
