"""Add storyboard draft tables.

Revision ID: ecdbb9f876f8
Revises: 36bf151da189
Create Date: 2026-08-13 20:13:53.834033

"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "ecdbb9f876f8"
down_revision: str | Sequence[str] | None = "36bf151da189"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.drop_constraint(
        "ck_sbd_asset_ref_binding_source",
        "sbd_asset_references",
        type_="check",
    )
    op.create_check_constraint(
        "ck_sbd_asset_ref_binding_source",
        "sbd_asset_references",
        "binding_source IN ('manual', 'ai')",
    )
    op.create_table(
        "sbd_draft_batches",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("project_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("input_script_version_id", sa.Uuid(), nullable=False),
        sa.Column("narrative_structure_id", sa.Uuid(), nullable=False),
        sa.Column("narrative_revision", sa.Integer(), nullable=False),
        sa.Column("narrative_dependency_hash", sa.String(length=64), nullable=False),
        sa.Column("input_hash", sa.String(length=64), nullable=False),
        sa.Column("target_duration_ms", sa.Integer(), nullable=False),
        sa.Column("aspect_ratio", sa.String(length=10), nullable=False),
        sa.Column("visual_style", sa.String(length=200), nullable=True),
        sa.Column("engine_version", sa.String(length=80), nullable=False),
        sa.Column("model_name", sa.String(length=160), nullable=False),
        sa.Column("prompt_version", sa.String(length=80), nullable=False),
        sa.Column("schema_version", sa.String(length=80), nullable=False),
        sa.Column("base_order_hash", sa.String(length=64), nullable=False),
        sa.Column("base_shot_hash", sa.String(length=64), nullable=False),
        sa.Column("base_shots", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("task_id", sa.Uuid(), nullable=True),
        sa.Column("status", sa.String(length=30), nullable=False),
        sa.Column("provider_result_hash", sa.String(length=64), nullable=True),
        sa.Column("error_code", sa.String(length=80), nullable=True),
        sa.Column("approve_idempotency_key", sa.String(length=200), nullable=True),
        sa.Column("approve_command_hash", sa.String(length=64), nullable=True),
        sa.Column("apply_idempotency_key", sa.String(length=200), nullable=True),
        sa.Column("apply_command_hash", sa.String(length=64), nullable=True),
        sa.Column("apply_result", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("revision", sa.Integer(), nullable=False),
        sa.Column("idempotency_key", sa.String(length=200), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "status IN ('queued', 'running', 'needs_review', 'approved', "
            "'applied', 'failed', 'unknown', 'cancelled')",
            name="ck_sbd_draft_batch_status",
        ),
        sa.CheckConstraint("narrative_revision >= 1", name="ck_sbd_draft_narrative_revision"),
        sa.CheckConstraint("revision >= 1", name="ck_sbd_draft_batch_revision"),
        sa.CheckConstraint(
            "target_duration_ms >= 1000 AND target_duration_ms <= 7200000",
            name="ck_sbd_draft_batch_duration",
        ),
        sa.ForeignKeyConstraint(
            ["created_by"],
            ["idn_user_accounts.id"],
        ),
        sa.ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_sbd_draft_batch_episode",
        ),
        sa.ForeignKeyConstraint(
            ["input_script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_sbd_draft_batch_script",
        ),
        sa.ForeignKeyConstraint(
            ["narrative_structure_id", "input_script_version_id", "episode_id", "workspace_id"],
            [
                "scr_narrative_structures.id",
                "scr_narrative_structures.script_version_id",
                "scr_narrative_structures.episode_id",
                "scr_narrative_structures.workspace_id",
            ],
            name="fk_sbd_draft_batch_narrative",
        ),
        sa.ForeignKeyConstraint(
            ["project_id", "workspace_id"],
            ["prj_projects.id", "prj_projects.workspace_id"],
            name="fk_sbd_draft_batch_project",
        ),
        sa.ForeignKeyConstraint(
            ["task_id", "workspace_id"],
            ["prod_tasks.id", "prod_tasks.workspace_id"],
            name="fk_sbd_draft_batch_task",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("episode_id", "idempotency_key", name="uq_sbd_draft_batch_idempotency"),
        sa.UniqueConstraint("id", "episode_id", "workspace_id", name="uq_sbd_draft_batch_scope"),
        sa.UniqueConstraint("id", "workspace_id", name="uq_sbd_draft_batch_workspace"),
        sa.UniqueConstraint("task_id", name="uq_sbd_draft_batch_task"),
        sa.UniqueConstraint(
            "workspace_id", "apply_idempotency_key", name="uq_sbd_draft_apply_idempotency"
        ),
        sa.UniqueConstraint(
            "workspace_id", "approve_idempotency_key", name="uq_sbd_draft_approve_idempotency"
        ),
    )
    op.create_index(
        "ix_sbd_draft_episode_created",
        "sbd_draft_batches",
        ["episode_id", "created_at"],
        unique=False,
    )
    op.create_index(
        "ix_sbd_draft_workspace_status",
        "sbd_draft_batches",
        ["workspace_id", "status"],
        unique=False,
    )
    op.create_table(
        "sbd_draft_input_assets",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("batch_id", sa.Uuid(), nullable=False),
        sa.Column("asset_id", sa.Uuid(), nullable=False),
        sa.Column("asset_state_id", sa.Uuid(), nullable=False),
        sa.Column("asset_version_id", sa.Uuid(), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("kind", sa.String(length=30), nullable=False),
        sa.Column("name", sa.String(length=200), nullable=False),
        sa.Column("state_label", sa.String(length=120), nullable=False),
        sa.Column("state_revision", sa.Integer(), nullable=False),
        sa.Column("readiness_hash", sa.String(length=64), nullable=False),
        sa.CheckConstraint("position >= 1", name="ck_sbd_draft_asset_position"),
        sa.CheckConstraint("state_revision >= 1", name="ck_sbd_draft_asset_revision"),
        sa.ForeignKeyConstraint(
            ["asset_version_id", "asset_state_id", "asset_id", "workspace_id"],
            [
                "ast_asset_versions.id",
                "ast_asset_versions.asset_state_id",
                "ast_asset_versions.asset_id",
                "ast_asset_versions.workspace_id",
            ],
            name="fk_sbd_draft_asset_version",
        ),
        sa.ForeignKeyConstraint(
            ["batch_id", "workspace_id"],
            ["sbd_draft_batches.id", "sbd_draft_batches.workspace_id"],
            name="fk_sbd_draft_asset_batch",
            ondelete="CASCADE",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("batch_id", "asset_state_id", name="uq_sbd_draft_asset_state"),
        sa.UniqueConstraint(
            "batch_id", "asset_version_id", "workspace_id", name="uq_sbd_draft_asset_input"
        ),
        sa.UniqueConstraint("batch_id", "position", name="uq_sbd_draft_asset_position"),
    )
    op.create_table(
        "sbd_draft_shots",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("batch_id", sa.Uuid(), nullable=False),
        sa.Column("proposal_key", sa.String(length=120), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("title", sa.String(length=200), nullable=False),
        sa.Column("spec", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("content_hash", sa.String(length=64), nullable=False),
        sa.Column("risk_codes", postgresql.ARRAY(sa.String(length=80)), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("position >= 1 AND position <= 120", name="ck_sbd_draft_shot_position"),
        sa.ForeignKeyConstraint(
            ["batch_id", "workspace_id"],
            ["sbd_draft_batches.id", "sbd_draft_batches.workspace_id"],
            name="fk_sbd_draft_shot_batch",
            ondelete="CASCADE",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("batch_id", "position", name="uq_sbd_draft_shot_position"),
        sa.UniqueConstraint("batch_id", "proposal_key", name="uq_sbd_draft_shot_key"),
        sa.UniqueConstraint("id", "batch_id", "workspace_id", name="uq_sbd_draft_shot_scope"),
        sa.UniqueConstraint("id", "workspace_id", name="uq_sbd_draft_shot_workspace"),
    )
    op.create_table(
        "sbd_draft_asset_refs",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("batch_id", sa.Uuid(), nullable=False),
        sa.Column("draft_shot_id", sa.Uuid(), nullable=False),
        sa.Column("slot_key", sa.String(length=100), nullable=False),
        sa.Column("role", sa.String(length=30), nullable=False),
        sa.Column("asset_version_id", sa.Uuid(), nullable=False),
        sa.Column("subject_key", sa.String(length=100), nullable=True),
        sa.CheckConstraint(
            "role IN ('location', 'character', 'prop', 'costume', 'visual_style', 'voice')",
            name="ck_sbd_draft_ref_role",
        ),
        sa.ForeignKeyConstraint(
            ["batch_id", "asset_version_id", "workspace_id"],
            [
                "sbd_draft_input_assets.batch_id",
                "sbd_draft_input_assets.asset_version_id",
                "sbd_draft_input_assets.workspace_id",
            ],
            name="fk_sbd_draft_ref_input",
        ),
        sa.ForeignKeyConstraint(
            ["draft_shot_id", "batch_id", "workspace_id"],
            ["sbd_draft_shots.id", "sbd_draft_shots.batch_id", "sbd_draft_shots.workspace_id"],
            name="fk_sbd_draft_ref_shot",
            ondelete="CASCADE",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("draft_shot_id", "slot_key", name="uq_sbd_draft_ref_slot"),
    )
    op.create_table(
        "sbd_draft_decisions",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("batch_id", sa.Uuid(), nullable=False),
        sa.Column("draft_shot_id", sa.Uuid(), nullable=False),
        sa.Column("sequence", sa.Integer(), nullable=False),
        sa.Column("action", sa.String(length=20), nullable=False),
        sa.Column(
            "target", postgresql.JSONB(none_as_null=True, astext_type=sa.Text()), nullable=True
        ),
        sa.Column("command_hash", sa.String(length=64), nullable=False),
        sa.Column("idempotency_key", sa.String(length=200), nullable=False),
        sa.Column("actor_id", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "(action = 'modified') = (target IS NOT NULL)", name="ck_sbd_draft_decision_target"
        ),
        sa.CheckConstraint(
            "action IN ('accepted', 'modified', 'ignored')", name="ck_sbd_draft_decision_action"
        ),
        sa.CheckConstraint("sequence >= 1", name="ck_sbd_draft_decision_sequence"),
        sa.ForeignKeyConstraint(
            ["actor_id"],
            ["idn_user_accounts.id"],
        ),
        sa.ForeignKeyConstraint(
            ["draft_shot_id", "batch_id", "workspace_id"],
            ["sbd_draft_shots.id", "sbd_draft_shots.batch_id", "sbd_draft_shots.workspace_id"],
            name="fk_sbd_draft_decision_shot",
            ondelete="CASCADE",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("batch_id", "sequence", name="uq_sbd_draft_decision_sequence"),
        sa.UniqueConstraint(
            "workspace_id", "idempotency_key", name="uq_sbd_draft_decision_idempotency"
        ),
    )
    op.create_index(
        "ix_sbd_draft_decision_shot",
        "sbd_draft_decisions",
        ["draft_shot_id", "sequence"],
        unique=False,
    )
    op.create_table(
        "sbd_draft_input_units",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("batch_id", sa.Uuid(), nullable=False),
        sa.Column("narrative_unit_id", sa.Uuid(), nullable=False),
        sa.Column("unit_version_id", sa.Uuid(), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("kind", sa.String(length=30), nullable=False),
        sa.Column("exact_text", sa.Text(), nullable=False),
        sa.Column("text_hash", sa.String(length=64), nullable=False),
        sa.Column("required_for_coverage", sa.Boolean(), nullable=False),
        sa.Column("source_scene_id", sa.Uuid(), nullable=True),
        sa.Column("source_dialogue_id", sa.Uuid(), nullable=True),
        sa.CheckConstraint("position >= 1", name="ck_sbd_draft_unit_position"),
        sa.ForeignKeyConstraint(
            ["batch_id", "episode_id", "workspace_id"],
            [
                "sbd_draft_batches.id",
                "sbd_draft_batches.episode_id",
                "sbd_draft_batches.workspace_id",
            ],
            name="fk_sbd_draft_unit_batch",
            ondelete="CASCADE",
        ),
        sa.ForeignKeyConstraint(
            ["unit_version_id", "narrative_unit_id", "episode_id", "workspace_id"],
            [
                "scr_narrative_unit_versions.id",
                "scr_narrative_unit_versions.unit_id",
                "scr_narrative_unit_versions.episode_id",
                "scr_narrative_unit_versions.workspace_id",
            ],
            name="fk_sbd_draft_unit_version",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("batch_id", "position", name="uq_sbd_draft_unit_position"),
        sa.UniqueConstraint(
            "batch_id", "unit_version_id", "workspace_id", name="uq_sbd_draft_unit_input"
        ),
    )
    op.create_table(
        "sbd_draft_shot_units",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("batch_id", sa.Uuid(), nullable=False),
        sa.Column("draft_shot_id", sa.Uuid(), nullable=False),
        sa.Column("unit_version_id", sa.Uuid(), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.CheckConstraint("position >= 1", name="ck_sbd_draft_shot_unit_position"),
        sa.ForeignKeyConstraint(
            ["batch_id", "unit_version_id", "workspace_id"],
            [
                "sbd_draft_input_units.batch_id",
                "sbd_draft_input_units.unit_version_id",
                "sbd_draft_input_units.workspace_id",
            ],
            name="fk_sbd_draft_shot_unit_input",
        ),
        sa.ForeignKeyConstraint(
            ["draft_shot_id", "batch_id", "workspace_id"],
            ["sbd_draft_shots.id", "sbd_draft_shots.batch_id", "sbd_draft_shots.workspace_id"],
            name="fk_sbd_draft_shot_unit_shot",
            ondelete="CASCADE",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("draft_shot_id", "position", name="uq_sbd_draft_shot_unit_position"),
        sa.UniqueConstraint("draft_shot_id", "unit_version_id", name="uq_sbd_draft_shot_unit"),
    )
    op.add_column("sbd_shots", sa.Column("source_draft_shot_id", sa.Uuid(), nullable=True))
    op.create_check_constraint(
        "ck_sbd_shot_single_origin",
        "sbd_shots",
        "source_candidate_id IS NULL OR source_draft_shot_id IS NULL",
    )
    op.create_index(
        "uq_sbd_shot_workspace_draft",
        "sbd_shots",
        ["workspace_id", "source_draft_shot_id"],
        unique=True,
        postgresql_where=sa.text("source_draft_shot_id IS NOT NULL"),
    )
    op.create_foreign_key(
        "fk_sbd_shot_draft_workspace",
        "sbd_shots",
        "sbd_draft_shots",
        ["source_draft_shot_id", "workspace_id"],
        ["id", "workspace_id"],
    )


def downgrade() -> None:
    op.drop_constraint("fk_sbd_shot_draft_workspace", "sbd_shots", type_="foreignkey")
    op.drop_constraint("ck_sbd_shot_single_origin", "sbd_shots", type_="check")
    op.drop_index(
        "uq_sbd_shot_workspace_draft",
        table_name="sbd_shots",
        postgresql_where=sa.text("source_draft_shot_id IS NOT NULL"),
    )
    op.drop_column("sbd_shots", "source_draft_shot_id")
    op.drop_table("sbd_draft_shot_units")
    op.drop_table("sbd_draft_input_units")
    op.drop_index("ix_sbd_draft_decision_shot", table_name="sbd_draft_decisions")
    op.drop_table("sbd_draft_decisions")
    op.drop_table("sbd_draft_asset_refs")
    op.drop_table("sbd_draft_shots")
    op.drop_table("sbd_draft_input_assets")
    op.drop_index("ix_sbd_draft_workspace_status", table_name="sbd_draft_batches")
    op.drop_index("ix_sbd_draft_episode_created", table_name="sbd_draft_batches")
    op.drop_table("sbd_draft_batches")
    op.drop_constraint(
        "ck_sbd_asset_ref_binding_source",
        "sbd_asset_references",
        type_="check",
    )
    op.create_check_constraint(
        "ck_sbd_asset_ref_binding_source",
        "sbd_asset_references",
        "binding_source = 'manual'",
    )
