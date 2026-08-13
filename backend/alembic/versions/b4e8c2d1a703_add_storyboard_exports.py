"""Add trusted storyboard exports.

Revision ID: b4e8c2d1a703
Revises: 7a2d9c4e6f10
Create Date: 2026-08-14 02:30:00

"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "b4e8c2d1a703"
down_revision: str | Sequence[str] | None = "7a2d9c4e6f10"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_table(
        "med_media_lineages",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("media_version_id", sa.Uuid(), nullable=False),
        sa.Column("source_type", sa.String(length=40), nullable=False),
        sa.Column("source_id", sa.Uuid(), nullable=False),
        sa.Column("source_hash", sa.String(length=64), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("position >= 1", name="ck_med_lineage_position"),
        sa.CheckConstraint(
            "source_type IN ('asset_version', 'narrative_unit_version', "
            "'script_version', 'shot_spec_version', 'storyboard_coverage', "
            "'storyboard_export_snapshot', 'storyboard_readiness')",
            name="ck_med_lineage_source_type",
        ),
        sa.ForeignKeyConstraint(
            ["media_version_id", "workspace_id"],
            ["med_media_versions.id", "med_media_versions.workspace_id"],
            name="fk_med_lineage_version_workspace",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "media_version_id",
            "position",
            name="uq_med_lineage_version_position",
        ),
        sa.UniqueConstraint(
            "media_version_id",
            "source_type",
            "source_id",
            name="uq_med_lineage_source",
        ),
    )
    op.create_index(
        "ix_med_lineage_source",
        "med_media_lineages",
        ["source_type", "source_id"],
    )
    op.create_table(
        "sbd_export_jobs",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("project_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("schema_version", sa.Integer(), nullable=False),
        sa.Column("input_hash", sa.String(length=64), nullable=False),
        sa.Column("input_snapshot", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("command_hash", sa.String(length=64), nullable=False),
        sa.Column("idempotency_key", sa.String(length=200), nullable=False),
        sa.Column("task_id", sa.Uuid(), nullable=True),
        sa.Column("status", sa.String(length=20), nullable=False),
        sa.Column("error_code", sa.String(length=80), nullable=True),
        sa.Column("error_summary", sa.Text(), nullable=True),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("schema_version = 1", name="ck_sbd_export_job_schema"),
        sa.CheckConstraint(
            "status IN ('queued', 'running', 'succeeded', 'failed')",
            name="ck_sbd_export_job_status",
        ),
        sa.ForeignKeyConstraint(["created_by"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_sbd_export_job_episode",
        ),
        sa.ForeignKeyConstraint(
            ["project_id", "workspace_id"],
            ["prj_projects.id", "prj_projects.workspace_id"],
            name="fk_sbd_export_job_project",
        ),
        sa.ForeignKeyConstraint(
            ["task_id", "workspace_id"],
            ["prod_tasks.id", "prod_tasks.workspace_id"],
            name="fk_sbd_export_job_task",
            deferrable=True,
            initially="DEFERRED",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "episode_id",
            "idempotency_key",
            name="uq_sbd_export_job_idempotency",
        ),
        sa.UniqueConstraint(
            "id",
            "episode_id",
            "workspace_id",
            name="uq_sbd_export_job_scope",
        ),
        sa.UniqueConstraint("id", "workspace_id", name="uq_sbd_export_job_workspace"),
        sa.UniqueConstraint("task_id", name="uq_sbd_export_job_task"),
    )
    op.create_index(
        "ix_sbd_export_job_episode_created",
        "sbd_export_jobs",
        ["episode_id", "created_at"],
    )
    op.create_index(
        "ix_sbd_export_job_workspace_status",
        "sbd_export_jobs",
        ["workspace_id", "status"],
    )
    op.create_table(
        "sbd_export_manifests",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("job_id", sa.Uuid(), nullable=False),
        sa.Column("schema_version", sa.Integer(), nullable=False),
        sa.Column("input_hash", sa.String(length=64), nullable=False),
        sa.Column("input_snapshot", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("file_manifest", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("media_version_id", sa.Uuid(), nullable=False),
        sa.Column("package_sha256", sa.String(length=64), nullable=False),
        sa.Column("package_size_bytes", sa.BigInteger(), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("schema_version = 1", name="ck_sbd_export_manifest_schema"),
        sa.CheckConstraint(
            "package_size_bytes >= 1",
            name="ck_sbd_export_manifest_size",
        ),
        sa.ForeignKeyConstraint(["created_by"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(
            ["job_id", "episode_id", "workspace_id"],
            [
                "sbd_export_jobs.id",
                "sbd_export_jobs.episode_id",
                "sbd_export_jobs.workspace_id",
            ],
            name="fk_sbd_export_manifest_job",
        ),
        sa.ForeignKeyConstraint(
            ["media_version_id", "workspace_id"],
            ["med_media_versions.id", "med_media_versions.workspace_id"],
            name="fk_sbd_export_manifest_media",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("job_id", name="uq_sbd_export_manifest_job"),
        sa.UniqueConstraint(
            "media_version_id",
            name="uq_sbd_export_manifest_media",
        ),
        sa.UniqueConstraint(
            "id",
            "workspace_id",
            name="uq_sbd_export_manifest_workspace",
        ),
    )
    op.create_index(
        "ix_sbd_export_manifest_episode_created",
        "sbd_export_manifests",
        ["episode_id", "created_at"],
    )


def downgrade() -> None:
    op.drop_index(
        "ix_sbd_export_manifest_episode_created",
        table_name="sbd_export_manifests",
    )
    op.drop_table("sbd_export_manifests")
    op.drop_index(
        "ix_sbd_export_job_workspace_status",
        table_name="sbd_export_jobs",
    )
    op.drop_index(
        "ix_sbd_export_job_episode_created",
        table_name="sbd_export_jobs",
    )
    op.drop_table("sbd_export_jobs")
    op.drop_index("ix_med_lineage_source", table_name="med_media_lineages")
    op.drop_table("med_media_lineages")
