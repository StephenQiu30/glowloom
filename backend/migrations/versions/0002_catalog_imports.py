"""Add idempotent catalog import persistence."""

from typing import Any

import sqlalchemy as sa
from alembic import op
from sqlalchemy.dialects import postgresql


revision = "platform_0002"
down_revision = "platform_0001"
branch_labels = None
depends_on = None


def upgrade() -> None:
    op.add_column(
        "prompt_templates",
        sa.Column(
            "parameters",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
            server_default=sa.text("'{}'::jsonb"),
        ),
        schema="catalog",
    )
    op.alter_column(
        "prompt_templates",
        "published_at",
        existing_type=sa.DateTime(timezone=True),
        nullable=True,
        schema="catalog",
    )
    _create_import_manifests()
    _create_search_documents()


def downgrade() -> None:
    op.drop_table("search_documents", schema="catalog")
    op.drop_table("import_manifests", schema="catalog")
    op.execute(
        "UPDATE catalog.prompt_templates "
        "SET published_at = collected_at WHERE published_at IS NULL"
    )
    op.alter_column(
        "prompt_templates",
        "published_at",
        existing_type=sa.DateTime(timezone=True),
        nullable=False,
        schema="catalog",
    )
    op.drop_column("prompt_templates", "parameters", schema="catalog")


def _uuid(name: str, **kwargs: Any) -> sa.Column[Any]:
    return sa.Column(name, postgresql.UUID(as_uuid=True), **kwargs)


def _create_import_manifests() -> None:
    op.create_table(
        "import_manifests",
        _uuid("id", primary_key=True),
        sa.Column("source_name", sa.String(128), nullable=False),
        sa.Column("source_url", sa.Text(), nullable=False),
        sa.Column("source_revision", sa.String(128), nullable=False),
        sa.Column("source_license", sa.String(128), nullable=False),
        sa.Column("collected_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("item_count", sa.Integer(), nullable=False),
        sa.Column("checksum", sa.String(64), nullable=False),
        sa.Column(
            "imported_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.func.now(),
        ),
        sa.CheckConstraint(
            "item_count > 0", name="ck_catalog_import_manifests_item_count"
        ),
        sa.CheckConstraint(
            "checksum ~ '^[0-9a-f]{64}$'",
            name="ck_catalog_import_manifests_checksum",
        ),
        sa.UniqueConstraint(
            "source_name",
            "source_revision",
            "checksum",
            name="uq_catalog_import_manifests_source_revision_checksum",
        ),
        schema="catalog",
    )


def _create_search_documents() -> None:
    op.create_table(
        "search_documents",
        _uuid("template_id", primary_key=True),
        sa.Column("search_text", sa.Text(), nullable=False),
        sa.Column(
            "search_vector",
            postgresql.TSVECTOR(),
            sa.Computed(
                "to_tsvector('simple'::regconfig, search_text)", persisted=True
            ),
            nullable=False,
        ),
        sa.ForeignKeyConstraint(
            ["template_id"],
            ["catalog.prompt_templates.id"],
            name="fk_catalog_search_documents_template_id_prompt_templates",
            ondelete="CASCADE",
        ),
        schema="catalog",
    )
    op.create_index(
        "ix_catalog_search_documents_search_vector",
        "search_documents",
        ["search_vector"],
        unique=False,
        schema="catalog",
        postgresql_using="gin",
    )
