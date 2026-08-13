"""Add asset state and occurrence tables.

Revision ID: 6c1f8d4a7e20
Revises: 2b7e4c9a1d63
Create Date: 2026-08-13 18:40:00.000000

"""

from collections.abc import Sequence
from uuid import uuid4

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "6c1f8d4a7e20"
down_revision: str | Sequence[str] | None = "2b7e4c9a1d63"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def _backfill_base_states() -> None:
    bind = op.get_bind()
    assets = bind.execute(
        sa.text(
            """
            SELECT id, workspace_id, current_version_id, created_by, created_at, updated_at
            FROM ast_assets
            ORDER BY created_at, id
            """
        )
    ).mappings()
    for asset in assets:
        state_id = uuid4()
        bind.execute(
            sa.text(
                """
                INSERT INTO ast_asset_states (
                    id, workspace_id, asset_id, state_key, label, description,
                    status, current_version_id, revision, creation_key,
                    command_receipts, created_by, created_at, updated_at
                ) VALUES (
                    :id, :workspace_id, :asset_id, 'base', '基础状态', '',
                    'active', :current_version_id, 1, 'base',
                    CAST('{}' AS jsonb), :created_by, :created_at, :updated_at
                )
                """
            ),
            {
                "id": state_id,
                "workspace_id": asset["workspace_id"],
                "asset_id": asset["id"],
                "current_version_id": asset["current_version_id"],
                "created_by": asset["created_by"],
                "created_at": asset["created_at"],
                "updated_at": asset["updated_at"],
            },
        )
        bind.execute(
            sa.text(
                "UPDATE ast_asset_versions SET asset_state_id = :state_id "
                "WHERE asset_id = :asset_id"
            ),
            {"state_id": state_id, "asset_id": asset["id"]},
        )


def upgrade() -> None:
    op.add_column(
        "ast_asset_versions",
        sa.Column("asset_state_id", sa.Uuid(), nullable=True),
    )
    op.create_table(
        "ast_asset_states",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("asset_id", sa.Uuid(), nullable=False),
        sa.Column("state_key", sa.String(length=80), nullable=False),
        sa.Column("label", sa.String(length=120), nullable=False),
        sa.Column("description", sa.Text(), nullable=False),
        sa.Column("status", sa.String(length=20), nullable=False),
        sa.Column("current_version_id", sa.Uuid(), nullable=True),
        sa.Column("revision", sa.Integer(), nullable=False),
        sa.Column("creation_key", sa.String(length=200), nullable=False),
        sa.Column("command_receipts", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("revision >= 1", name="ck_ast_state_revision"),
        sa.CheckConstraint(
            "status IN ('active', 'disabled')",
            name="ck_ast_state_status",
        ),
        sa.ForeignKeyConstraint(
            ["asset_id", "workspace_id"],
            ["ast_assets.id", "ast_assets.workspace_id"],
            name="fk_ast_state_asset_workspace",
        ),
        sa.ForeignKeyConstraint(["created_by"], ["idn_user_accounts.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "asset_id",
            "creation_key",
            name="uq_ast_state_asset_creation",
        ),
        sa.UniqueConstraint(
            "asset_id",
            "state_key",
            name="uq_ast_state_asset_key",
        ),
        sa.UniqueConstraint(
            "id",
            "workspace_id",
            name="uq_ast_state_id_workspace",
        ),
        sa.UniqueConstraint(
            "id",
            "asset_id",
            "workspace_id",
            name="uq_ast_state_scope",
        ),
    )
    op.create_index(
        "ix_ast_state_asset_status",
        "ast_asset_states",
        ["asset_id", "status"],
        unique=False,
    )

    _backfill_base_states()
    op.execute("SET CONSTRAINTS ALL IMMEDIATE")

    op.alter_column("ast_asset_versions", "asset_state_id", nullable=False)
    op.create_unique_constraint(
        "uq_ast_version_scope",
        "ast_asset_versions",
        ["id", "asset_state_id", "asset_id", "workspace_id"],
    )
    op.create_index(
        "ix_ast_version_state_number",
        "ast_asset_versions",
        ["asset_state_id", "version_no"],
        unique=False,
    )
    op.create_foreign_key(
        "fk_ast_version_state_scope",
        "ast_asset_versions",
        "ast_asset_states",
        ["asset_state_id", "asset_id", "workspace_id"],
        ["id", "asset_id", "workspace_id"],
        deferrable=True,
        initially="DEFERRED",
    )
    op.drop_constraint(
        "fk_ast_version_asset_workspace",
        "ast_asset_versions",
        type_="foreignkey",
    )
    op.create_foreign_key(
        "fk_ast_state_current_version_scope",
        "ast_asset_states",
        "ast_asset_versions",
        ["current_version_id", "id", "asset_id", "workspace_id"],
        ["id", "asset_state_id", "asset_id", "workspace_id"],
        deferrable=True,
        initially="DEFERRED",
    )

    op.drop_constraint(
        "ck_ast_version_source_type",
        "ast_asset_versions",
        type_="check",
    )
    op.execute(
        "UPDATE ast_asset_versions "
        "SET source_type = 'script_extraction_candidate' "
        "WHERE source_type = 'candidate'"
    )
    op.create_check_constraint(
        "ck_ast_version_source_type",
        "ast_asset_versions",
        "source_type IN ('manual', 'script_extraction_candidate')",
    )

    op.create_unique_constraint(
        "uq_scr_narrative_version_unit_scope",
        "scr_narrative_unit_versions",
        ["id", "unit_id", "episode_id", "workspace_id"],
    )
    op.create_table(
        "ast_asset_occurrences",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("asset_state_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("narrative_unit_id", sa.Uuid(), nullable=False),
        sa.Column("narrative_unit_version_id", sa.Uuid(), nullable=False),
        sa.Column("sequence", sa.Integer(), nullable=False),
        sa.Column("decision", sa.String(length=20), nullable=False),
        sa.Column("origin", sa.String(length=30), nullable=False),
        sa.Column("evidence_hash", sa.String(length=64), nullable=False),
        sa.Column("idempotency_key", sa.String(length=200), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "decision IN ('link', 'unlink')",
            name="ck_ast_occurrence_decision",
        ),
        sa.CheckConstraint(
            "origin IN ('manual', 'script_candidate')",
            name="ck_ast_occurrence_origin",
        ),
        sa.CheckConstraint("sequence >= 1", name="ck_ast_occurrence_sequence"),
        sa.ForeignKeyConstraint(
            ["asset_state_id", "workspace_id"],
            ["ast_asset_states.id", "ast_asset_states.workspace_id"],
            name="fk_ast_occurrence_state_workspace",
        ),
        sa.ForeignKeyConstraint(["created_by"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(
            ["narrative_unit_id", "episode_id", "workspace_id"],
            [
                "scr_narrative_units.id",
                "scr_narrative_units.episode_id",
                "scr_narrative_units.workspace_id",
            ],
            name="fk_ast_occurrence_unit_scope",
        ),
        sa.ForeignKeyConstraint(
            [
                "narrative_unit_version_id",
                "narrative_unit_id",
                "episode_id",
                "workspace_id",
            ],
            [
                "scr_narrative_unit_versions.id",
                "scr_narrative_unit_versions.unit_id",
                "scr_narrative_unit_versions.episode_id",
                "scr_narrative_unit_versions.workspace_id",
            ],
            name="fk_ast_occurrence_unit_version_scope",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "asset_state_id",
            "idempotency_key",
            name="uq_ast_occurrence_state_idempotency",
        ),
        sa.UniqueConstraint(
            "asset_state_id",
            "sequence",
            name="uq_ast_occurrence_state_sequence",
        ),
    )
    op.create_index(
        "ix_ast_occurrence_episode_state",
        "ast_asset_occurrences",
        ["episode_id", "asset_state_id"],
        unique=False,
    )
    op.create_index(
        "ix_ast_occurrence_unit_created",
        "ast_asset_occurrences",
        ["narrative_unit_id", "created_at"],
        unique=False,
    )

    op.add_column(
        "sbd_asset_references",
        sa.Column("asset_state_id", sa.Uuid(), nullable=True),
    )
    op.add_column(
        "sbd_asset_references",
        sa.Column("asset_id", sa.Uuid(), nullable=True),
    )
    op.add_column(
        "sbd_asset_references",
        sa.Column("binding_source", sa.String(length=30), nullable=True),
    )
    op.execute(
        """
        UPDATE sbd_asset_references AS reference
        SET asset_state_id = version.asset_state_id,
            asset_id = version.asset_id,
            binding_source = 'manual'
        FROM ast_asset_versions AS version
        WHERE version.id = reference.asset_version_id
        """
    )
    op.alter_column("sbd_asset_references", "asset_state_id", nullable=False)
    op.alter_column("sbd_asset_references", "asset_id", nullable=False)
    op.alter_column("sbd_asset_references", "binding_source", nullable=False)
    op.drop_constraint(
        "fk_sbd_asset_ref_version_workspace",
        "sbd_asset_references",
        type_="foreignkey",
    )
    op.create_foreign_key(
        "fk_sbd_asset_ref_version_scope",
        "sbd_asset_references",
        "ast_asset_versions",
        ["asset_version_id", "asset_state_id", "asset_id", "workspace_id"],
        ["id", "asset_state_id", "asset_id", "workspace_id"],
    )
    op.create_check_constraint(
        "ck_sbd_asset_ref_binding_source",
        "sbd_asset_references",
        "binding_source = 'manual'",
    )
    op.create_index(
        "ix_sbd_asset_ref_state",
        "sbd_asset_references",
        ["asset_state_id"],
        unique=False,
    )

    op.drop_constraint(
        "fk_ast_asset_current_version_workspace",
        "ast_assets",
        type_="foreignkey",
    )
    op.drop_column("ast_assets", "current_version_id")


def downgrade() -> None:
    bind = op.get_bind()
    occurrence_count = bind.scalar(sa.text("SELECT count(*) FROM ast_asset_occurrences"))
    non_base_count = bind.scalar(
        sa.text("SELECT count(*) FROM ast_asset_states WHERE state_key != 'base'")
    )
    if occurrence_count or non_base_count:
        raise RuntimeError(
            "asset state downgrade would discard occurrence or non-base state data; "
            "restore the pre-migration backup instead"
        )

    op.add_column(
        "ast_assets",
        sa.Column("current_version_id", sa.Uuid(), nullable=True),
    )
    op.execute(
        """
        UPDATE ast_assets AS asset
        SET current_version_id = state.current_version_id
        FROM ast_asset_states AS state
        WHERE state.asset_id = asset.id AND state.state_key = 'base'
        """
    )
    op.create_foreign_key(
        "fk_ast_asset_current_version_workspace",
        "ast_assets",
        "ast_asset_versions",
        ["current_version_id", "workspace_id"],
        ["id", "workspace_id"],
        deferrable=True,
        initially="DEFERRED",
    )

    op.drop_index("ix_sbd_asset_ref_state", table_name="sbd_asset_references")
    op.drop_constraint(
        "ck_sbd_asset_ref_binding_source",
        "sbd_asset_references",
        type_="check",
    )
    op.drop_constraint(
        "fk_sbd_asset_ref_version_scope",
        "sbd_asset_references",
        type_="foreignkey",
    )
    op.create_foreign_key(
        "fk_sbd_asset_ref_version_workspace",
        "sbd_asset_references",
        "ast_asset_versions",
        ["asset_version_id", "workspace_id"],
        ["id", "workspace_id"],
    )
    op.drop_column("sbd_asset_references", "binding_source")
    op.drop_column("sbd_asset_references", "asset_id")
    op.drop_column("sbd_asset_references", "asset_state_id")

    op.drop_index("ix_ast_occurrence_unit_created", table_name="ast_asset_occurrences")
    op.drop_index("ix_ast_occurrence_episode_state", table_name="ast_asset_occurrences")
    op.drop_table("ast_asset_occurrences")
    op.drop_constraint(
        "uq_scr_narrative_version_unit_scope",
        "scr_narrative_unit_versions",
        type_="unique",
    )

    op.drop_constraint(
        "fk_ast_state_current_version_scope",
        "ast_asset_states",
        type_="foreignkey",
    )
    op.drop_constraint(
        "fk_ast_version_state_scope",
        "ast_asset_versions",
        type_="foreignkey",
    )
    op.create_foreign_key(
        "fk_ast_version_asset_workspace",
        "ast_asset_versions",
        "ast_assets",
        ["asset_id", "workspace_id"],
        ["id", "workspace_id"],
        deferrable=True,
        initially="DEFERRED",
    )
    op.drop_index("ix_ast_version_state_number", table_name="ast_asset_versions")
    op.drop_constraint(
        "uq_ast_version_scope",
        "ast_asset_versions",
        type_="unique",
    )

    op.drop_constraint(
        "ck_ast_version_source_type",
        "ast_asset_versions",
        type_="check",
    )
    op.execute(
        "UPDATE ast_asset_versions SET source_type = 'candidate' "
        "WHERE source_type = 'script_extraction_candidate'"
    )
    op.create_check_constraint(
        "ck_ast_version_source_type",
        "ast_asset_versions",
        "source_type IN ('manual', 'candidate')",
    )

    op.drop_column("ast_asset_versions", "asset_state_id")
    op.drop_index("ix_ast_state_asset_status", table_name="ast_asset_states")
    op.drop_table("ast_asset_states")
