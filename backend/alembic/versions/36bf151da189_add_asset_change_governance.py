"""Add asset change governance.

Revision ID: 36bf151da189
Revises: 6c1f8d4a7e20
Create Date: 2026-08-13 18:15:22.529961
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "36bf151da189"
down_revision: str | Sequence[str] | None = "6c1f8d4a7e20"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.add_column(
        "ast_assets",
        sa.Column(
            "availability",
            sa.String(length=20),
            server_default="enabled",
            nullable=False,
        ),
    )
    op.add_column(
        "ast_assets",
        sa.Column(
            "name_revision",
            sa.Integer(),
            server_default="1",
            nullable=False,
        ),
    )
    op.add_column(
        "ast_assets",
        sa.Column(
            "command_receipts",
            postgresql.JSONB(astext_type=sa.Text()),
            server_default=sa.text("'{}'::jsonb"),
            nullable=False,
        ),
    )
    op.create_check_constraint(
        "ck_ast_asset_availability",
        "ast_assets",
        "availability IN ('enabled', 'disabled')",
    )
    op.create_check_constraint(
        "ck_ast_asset_name_revision",
        "ast_assets",
        "name_revision >= 1",
    )
    op.create_index(
        "ix_ast_asset_project_availability",
        "ast_assets",
        ["project_id", "availability"],
    )
    op.create_table(
        "ast_asset_name_revisions",
        sa.Column("asset_id", sa.Uuid(), nullable=False),
        sa.Column("revision_no", sa.Integer(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("name_snapshot", sa.String(length=200), nullable=False),
        sa.Column("normalized_name", sa.String(length=200), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "revision_no >= 1",
            name="ck_ast_name_revision_number",
        ),
        sa.ForeignKeyConstraint(
            ["asset_id", "workspace_id"],
            ["ast_assets.id", "ast_assets.workspace_id"],
            name="fk_ast_name_revision_asset_workspace",
            ondelete="CASCADE",
        ),
        sa.ForeignKeyConstraint(["created_by"], ["idn_user_accounts.id"]),
        sa.PrimaryKeyConstraint("asset_id", "revision_no"),
        sa.UniqueConstraint(
            "asset_id",
            "workspace_id",
            "revision_no",
            name="uq_ast_name_revision_scope",
        ),
    )
    op.create_index(
        "ix_ast_name_revision_asset_created",
        "ast_asset_name_revisions",
        ["asset_id", "created_at"],
    )
    op.execute(
        sa.text(
            """
            INSERT INTO ast_asset_name_revisions (
                asset_id,
                revision_no,
                workspace_id,
                name_snapshot,
                normalized_name,
                created_by,
                created_at
            )
            SELECT
                id,
                1,
                workspace_id,
                name,
                normalized_name,
                created_by,
                created_at
            FROM ast_assets
            """
        )
    )
    op.create_foreign_key(
        "fk_ast_asset_current_name",
        "ast_assets",
        "ast_asset_name_revisions",
        ["id", "workspace_id", "name_revision"],
        ["asset_id", "workspace_id", "revision_no"],
        initially="DEFERRED",
        deferrable=True,
        use_alter=True,
    )
    op.alter_column("ast_assets", "availability", server_default=None)
    op.alter_column("ast_assets", "name_revision", server_default=None)
    op.alter_column("ast_assets", "command_receipts", server_default=None)


def downgrade() -> None:
    op.drop_constraint(
        "fk_ast_asset_current_name",
        "ast_assets",
        type_="foreignkey",
    )
    op.drop_index(
        "ix_ast_name_revision_asset_created",
        table_name="ast_asset_name_revisions",
    )
    op.drop_table("ast_asset_name_revisions")
    op.drop_index(
        "ix_ast_asset_project_availability",
        table_name="ast_assets",
    )
    op.drop_constraint(
        "ck_ast_asset_name_revision",
        "ast_assets",
        type_="check",
    )
    op.drop_constraint(
        "ck_ast_asset_availability",
        "ast_assets",
        type_="check",
    )
    op.drop_column("ast_assets", "command_receipts")
    op.drop_column("ast_assets", "name_revision")
    op.drop_column("ast_assets", "availability")
