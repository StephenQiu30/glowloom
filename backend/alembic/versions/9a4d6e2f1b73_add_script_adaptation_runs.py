"""Add script adaptation runs.

Revision ID: 9a4d6e2f1b73
Revises: 7f3a9c1d2e84
Create Date: 2026-08-13 16:00:00.000000
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "9a4d6e2f1b73"
down_revision: str | Sequence[str] | None = "7f3a9c1d2e84"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "scr_adaptation_runs",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("source_id", sa.Uuid(), nullable=False),
        sa.Column("input_script_version_id", sa.Uuid(), nullable=False),
        sa.Column("input_hash", sa.String(length=64), nullable=False),
        sa.Column("target_duration_ms", sa.Integer(), nullable=False),
        sa.Column("core_plot_points", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("pacing", sa.String(length=20), nullable=False),
        sa.Column("colloquial_dialogue", sa.Boolean(), nullable=False),
        sa.Column("adaptation_engine_version", sa.String(length=80), nullable=False),
        sa.Column("model_name", sa.String(length=160), nullable=False),
        sa.Column("prompt_version", sa.String(length=80), nullable=False),
        sa.Column("schema_version", sa.String(length=80), nullable=False),
        sa.Column("task_id", sa.Uuid(), nullable=True),
        sa.Column("status", sa.String(length=30), nullable=False),
        sa.Column("candidate_body", sa.Text(), nullable=True),
        sa.Column("candidate_hash", sa.String(length=64), nullable=True),
        sa.Column("draft_body", sa.Text(), nullable=True),
        sa.Column("draft_hash", sa.String(length=64), nullable=True),
        sa.Column("change_summary", sa.Text(), nullable=True),
        sa.Column("estimated_duration_ms", sa.Integer(), nullable=True),
        sa.Column("error_code", sa.String(length=80), nullable=True),
        sa.Column("published_script_version_id", sa.Uuid(), nullable=True),
        sa.Column("publish_idempotency_key", sa.String(length=200), nullable=True),
        sa.Column("publish_command_hash", sa.String(length=64), nullable=True),
        sa.Column(
            "publish_result_snapshot",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
        ),
        sa.Column("cancel_idempotency_key", sa.String(length=200), nullable=True),
        sa.Column("cancel_command_hash", sa.String(length=64), nullable=True),
        sa.Column("revision", sa.Integer(), nullable=False),
        sa.Column("idempotency_key", sa.String(length=200), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "(candidate_body IS NULL) = (candidate_hash IS NULL)",
            name="ck_scr_adaptation_candidate_pair",
        ),
        sa.CheckConstraint(
            "(draft_body IS NULL) = (draft_hash IS NULL)",
            name="ck_scr_adaptation_draft_pair",
        ),
        sa.CheckConstraint(
            "estimated_duration_ms IS NULL OR "
            "(estimated_duration_ms >= 1000 AND estimated_duration_ms <= 600000)",
            name="ck_scr_adaptation_estimated_duration",
        ),
        sa.CheckConstraint(
            "pacing IN ('slow', 'balanced', 'fast')",
            name="ck_scr_adaptation_pacing",
        ),
        sa.CheckConstraint(
            "status != 'published' OR published_script_version_id IS NOT NULL",
            name="ck_scr_adaptation_published_version",
        ),
        sa.CheckConstraint("revision >= 1", name="ck_scr_adaptation_revision"),
        sa.CheckConstraint(
            "status IN ('queued', 'running', 'succeeded', 'published', "
            "'failed', 'cancelled', 'unknown')",
            name="ck_scr_adaptation_status",
        ),
        sa.CheckConstraint(
            "target_duration_ms >= 15000 AND target_duration_ms <= 600000",
            name="ck_scr_adaptation_target_duration",
        ),
        sa.ForeignKeyConstraint(
            ["created_by"],
            ["idn_user_accounts.id"],
            name=op.f("fk_scr_adaptation_runs_created_by_idn_user_accounts"),
        ),
        sa.ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_scr_adaptation_episode_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["input_script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_adaptation_input_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["published_script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_adaptation_published_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["source_id", "workspace_id"],
            ["scr_script_sources.id", "scr_script_sources.workspace_id"],
            name="fk_scr_adaptation_source_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["task_id", "workspace_id"],
            ["prod_tasks.id", "prod_tasks.workspace_id"],
            name="fk_scr_adaptation_task_workspace",
        ),
        sa.PrimaryKeyConstraint("id", name=op.f("pk_scr_adaptation_runs")),
        sa.UniqueConstraint(
            "episode_id",
            "idempotency_key",
            name="uq_scr_adaptation_episode_idempotency",
        ),
        sa.UniqueConstraint("id", "workspace_id", name="uq_scr_adaptation_id_workspace"),
        sa.UniqueConstraint("task_id", name="uq_scr_adaptation_task"),
        sa.UniqueConstraint(
            "workspace_id",
            "publish_idempotency_key",
            name="uq_scr_adaptation_publish_idempotency",
        ),
    )
    op.create_index(
        "ix_scr_adaptation_episode_created",
        "scr_adaptation_runs",
        ["episode_id", "created_at"],
        unique=False,
    )
    op.create_index(
        "ix_scr_adaptation_workspace_status_created",
        "scr_adaptation_runs",
        ["workspace_id", "status", "created_at"],
        unique=False,
    )


def downgrade() -> None:
    op.drop_index(
        "ix_scr_adaptation_workspace_status_created",
        table_name="scr_adaptation_runs",
    )
    op.drop_index("ix_scr_adaptation_episode_created", table_name="scr_adaptation_runs")
    op.drop_table("scr_adaptation_runs")
