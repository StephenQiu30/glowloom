"""增加整剧文档与格式分析表

Revision ID: 4c8e2f7a9b31
Revises: 8d9f2a6c4b71
Create Date: 2026-08-13
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "4c8e2f7a9b31"
down_revision: str | Sequence[str] | None = "8d9f2a6c4b71"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.drop_constraint(
        "ck_med_media_object_kind", "med_media_objects", type_="check"
    )
    op.create_check_constraint(
        "ck_med_media_object_kind",
        "med_media_objects",
        "kind IN ('image', 'video', 'audio', 'subtitle', 'delivery', 'document')",
    )
    op.drop_constraint("ck_med_upload_kind", "med_upload_sessions", type_="check")
    op.create_check_constraint(
        "ck_med_upload_kind",
        "med_upload_sessions",
        "declared_kind IN "
        "('image', 'video', 'audio', 'subtitle', 'delivery', 'document')",
    )
    op.create_table(
        "scr_script_documents",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("project_id", sa.Uuid(), nullable=False),
        sa.Column("title", sa.String(length=120), nullable=False),
        sa.Column("source_type", sa.String(length=20), nullable=False),
        sa.Column("source_media_version_id", sa.Uuid(), nullable=True),
        sa.Column("language", sa.String(length=35), nullable=False),
        sa.Column("rights_declaration", sa.Text(), nullable=False),
        sa.Column("input_hash", sa.String(length=64), nullable=False),
        sa.Column("status", sa.String(length=20), nullable=False),
        sa.Column("revision", sa.Integer(), nullable=False),
        sa.Column("idempotency_key", sa.String(length=200), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "(source_type = 'text' AND source_media_version_id IS NULL) OR "
            "(source_type = 'media' AND source_media_version_id IS NOT NULL)",
            name="ck_scr_document_source_reference",
        ),
        sa.CheckConstraint("revision >= 1", name="ck_scr_document_revision"),
        sa.CheckConstraint(
            "source_type IN ('text', 'media')",
            name="ck_scr_document_source_type",
        ),
        sa.CheckConstraint(
            "status IN ('active', 'archived')",
            name="ck_scr_document_status",
        ),
        sa.ForeignKeyConstraint(["created_by"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(
            ["project_id", "workspace_id"],
            ["prj_projects.id", "prj_projects.workspace_id"],
            name="fk_scr_document_project_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["source_media_version_id", "workspace_id"],
            ["med_media_versions.id", "med_media_versions.workspace_id"],
            name="fk_scr_document_media_workspace",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "id", "workspace_id", name="uq_scr_document_id_workspace"
        ),
        sa.UniqueConstraint(
            "project_id",
            "idempotency_key",
            name="uq_scr_document_project_idempotency",
        ),
    )
    op.create_index(
        "ix_scr_document_project_status_created",
        "scr_script_documents",
        ["project_id", "status", "created_at"],
        unique=False,
    )
    op.create_table(
        "scr_document_revisions",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("document_id", sa.Uuid(), nullable=False),
        sa.Column("version_no", sa.Integer(), nullable=False),
        sa.Column("source_type", sa.String(length=20), nullable=False),
        sa.Column("source_media_version_id", sa.Uuid(), nullable=True),
        sa.Column("raw_text", sa.Text(), nullable=False),
        sa.Column("raw_hash", sa.String(length=64), nullable=False),
        sa.Column("normalized_text", sa.Text(), nullable=False),
        sa.Column("normalized_hash", sa.String(length=64), nullable=False),
        sa.Column("normalizer_version", sa.String(length=80), nullable=False),
        sa.Column(
            "normalization_map",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
        ),
        sa.Column("codepoint_count", sa.Integer(), nullable=False),
        sa.Column("analysis_status", sa.String(length=30), nullable=False),
        sa.Column("analyzer_version", sa.String(length=80), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "analysis_status IN ('deterministic', 'ai_candidate_required', 'rejected')",
            name="ck_scr_document_revision_analysis_status",
        ),
        sa.CheckConstraint(
            "codepoint_count >= 1", name="ck_scr_document_revision_codepoints"
        ),
        sa.CheckConstraint(
            "(source_type = 'text' AND source_media_version_id IS NULL) OR "
            "(source_type = 'media' AND source_media_version_id IS NOT NULL)",
            name="ck_scr_document_revision_source_reference",
        ),
        sa.CheckConstraint(
            "source_type IN ('text', 'media')",
            name="ck_scr_document_revision_source_type",
        ),
        sa.CheckConstraint(
            "version_no >= 1", name="ck_scr_document_revision_number"
        ),
        sa.ForeignKeyConstraint(["created_by"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(
            ["document_id", "workspace_id"],
            ["scr_script_documents.id", "scr_script_documents.workspace_id"],
            name="fk_scr_document_revision_document_workspace",
            ondelete="CASCADE",
        ),
        sa.ForeignKeyConstraint(
            ["source_media_version_id", "workspace_id"],
            ["med_media_versions.id", "med_media_versions.workspace_id"],
            name="fk_scr_document_revision_media_workspace",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "id", "workspace_id", name="uq_scr_document_revision_id_workspace"
        ),
        sa.UniqueConstraint(
            "document_id",
            "version_no",
            name="uq_scr_document_revision_number",
        ),
    )
    op.create_index(
        "ix_scr_document_revision_document_created",
        "scr_document_revisions",
        ["document_id", "created_at"],
        unique=False,
    )
    op.create_index(
        "ix_scr_document_revision_raw_hash",
        "scr_document_revisions",
        ["raw_hash"],
        unique=False,
    )
    op.create_table(
        "scr_format_issues",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("document_revision_id", sa.Uuid(), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("code", sa.String(length=80), nullable=False),
        sa.Column("severity", sa.String(length=20), nullable=False),
        sa.Column("source_start", sa.Integer(), nullable=False),
        sa.Column("source_end", sa.Integer(), nullable=False),
        sa.Column("line_number", sa.Integer(), nullable=False),
        sa.Column("column_number", sa.Integer(), nullable=False),
        sa.Column("next_action", sa.String(length=100), nullable=False),
        sa.Column(
            "issue_details",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
        ),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "column_number >= 1", name="ck_scr_format_issue_column"
        ),
        sa.CheckConstraint("line_number >= 1", name="ck_scr_format_issue_line"),
        sa.CheckConstraint("position >= 1", name="ck_scr_format_issue_position"),
        sa.CheckConstraint(
            "severity IN ('warning', 'blocking')",
            name="ck_scr_format_issue_severity",
        ),
        sa.CheckConstraint(
            "source_end > source_start", name="ck_scr_format_issue_source_range"
        ),
        sa.CheckConstraint(
            "source_start >= 0", name="ck_scr_format_issue_source_start"
        ),
        sa.ForeignKeyConstraint(
            ["document_revision_id", "workspace_id"],
            ["scr_document_revisions.id", "scr_document_revisions.workspace_id"],
            name="fk_scr_format_issue_revision_workspace",
            ondelete="CASCADE",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "id", "workspace_id", name="uq_scr_format_issue_id_workspace"
        ),
        sa.UniqueConstraint(
            "document_revision_id",
            "position",
            name="uq_scr_format_issue_revision_position",
        ),
    )
    op.create_index(
        "ix_scr_format_issue_revision_severity",
        "scr_format_issues",
        ["document_revision_id", "severity"],
        unique=False,
    )
    op.create_table(
        "scr_narrative_blocks",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("document_revision_id", sa.Uuid(), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("kind", sa.String(length=30), nullable=False),
        sa.Column("source_start", sa.Integer(), nullable=False),
        sa.Column("source_end", sa.Integer(), nullable=False),
        sa.Column("text_hash", sa.String(length=64), nullable=False),
        sa.Column(
            "block_metadata",
            postgresql.JSONB(astext_type=sa.Text()),
            nullable=False,
        ),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "kind IN ('preamble', 'episode_marker', 'scene_heading', 'dialogue', "
            "'narration', 'action', 'separator')",
            name="ck_scr_narrative_block_kind",
        ),
        sa.CheckConstraint(
            "position >= 1", name="ck_scr_narrative_block_position"
        ),
        sa.CheckConstraint(
            "source_end > source_start", name="ck_scr_narrative_block_source_range"
        ),
        sa.CheckConstraint(
            "source_start >= 0", name="ck_scr_narrative_block_source_start"
        ),
        sa.ForeignKeyConstraint(
            ["document_revision_id", "workspace_id"],
            ["scr_document_revisions.id", "scr_document_revisions.workspace_id"],
            name="fk_scr_narrative_block_revision_workspace",
            ondelete="CASCADE",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "id", "workspace_id", name="uq_scr_narrative_block_id_workspace"
        ),
        sa.UniqueConstraint(
            "document_revision_id",
            "position",
            name="uq_scr_narrative_block_revision_position",
        ),
    )
    op.create_index(
        "ix_scr_narrative_block_revision_range",
        "scr_narrative_blocks",
        ["document_revision_id", "source_start"],
        unique=False,
    )


def downgrade() -> None:
    op.drop_index(
        "ix_scr_narrative_block_revision_range",
        table_name="scr_narrative_blocks",
    )
    op.drop_table("scr_narrative_blocks")
    op.drop_index(
        "ix_scr_format_issue_revision_severity", table_name="scr_format_issues"
    )
    op.drop_table("scr_format_issues")
    op.drop_index(
        "ix_scr_document_revision_raw_hash", table_name="scr_document_revisions"
    )
    op.drop_index(
        "ix_scr_document_revision_document_created",
        table_name="scr_document_revisions",
    )
    op.drop_table("scr_document_revisions")
    op.drop_index(
        "ix_scr_document_project_status_created",
        table_name="scr_script_documents",
    )
    op.drop_table("scr_script_documents")
    op.drop_constraint("ck_med_upload_kind", "med_upload_sessions", type_="check")
    op.create_check_constraint(
        "ck_med_upload_kind",
        "med_upload_sessions",
        "declared_kind IN ('image', 'video', 'audio', 'subtitle', 'delivery')",
    )
    op.drop_constraint(
        "ck_med_media_object_kind", "med_media_objects", type_="check"
    )
    op.create_check_constraint(
        "ck_med_media_object_kind",
        "med_media_objects",
        "kind IN ('image', 'video', 'audio', 'subtitle', 'delivery')",
    )
