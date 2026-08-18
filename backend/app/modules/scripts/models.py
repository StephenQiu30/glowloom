from datetime import UTC, datetime
from decimal import Decimal
from typing import Any
from uuid import UUID

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    DateTime,
    ForeignKey,
    ForeignKeyConstraint,
    Index,
    Integer,
    Numeric,
    String,
    Text,
    UniqueConstraint,
    Uuid,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class ScriptDocument(Base):
    __tablename__ = "scr_script_documents"
    __table_args__ = (
        ForeignKeyConstraint(
            ["project_id", "workspace_id"],
            ["prj_projects.id", "prj_projects.workspace_id"],
            name="fk_scr_document_project_workspace",
        ),
        ForeignKeyConstraint(
            ["source_media_version_id", "workspace_id"],
            ["med_media_versions.id", "med_media_versions.workspace_id"],
            name="fk_scr_document_media_workspace",
        ),
        CheckConstraint(
            "source_type IN ('text', 'media')",
            name="ck_scr_document_source_type",
        ),
        CheckConstraint(
            "(source_type = 'text' AND source_media_version_id IS NULL) OR "
            "(source_type = 'media' AND source_media_version_id IS NOT NULL)",
            name="ck_scr_document_source_reference",
        ),
        CheckConstraint("status IN ('active', 'archived')", name="ck_scr_document_status"),
        CheckConstraint("revision >= 1", name="ck_scr_document_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_document_id_workspace"),
        UniqueConstraint(
            "project_id",
            "idempotency_key",
            name="uq_scr_document_project_idempotency",
        ),
        Index(
            "ix_scr_document_project_status_created",
            "project_id",
            "status",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    title: Mapped[str] = mapped_column(String(120))
    source_type: Mapped[str] = mapped_column(String(20))
    source_media_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    language: Mapped[str] = mapped_column(String(35))
    rights_declaration: Mapped[str] = mapped_column(Text)
    input_hash: Mapped[str] = mapped_column(String(64))
    status: Mapped[str] = mapped_column(String(20), default="active")
    revision: Mapped[int] = mapped_column(Integer, default=1)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class DocumentRevision(Base):
    __tablename__ = "scr_document_revisions"
    __table_args__ = (
        ForeignKeyConstraint(
            ["document_id", "workspace_id"],
            ["scr_script_documents.id", "scr_script_documents.workspace_id"],
            name="fk_scr_document_revision_document_workspace",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ["source_media_version_id", "workspace_id"],
            ["med_media_versions.id", "med_media_versions.workspace_id"],
            name="fk_scr_document_revision_media_workspace",
        ),
        CheckConstraint("version_no >= 1", name="ck_scr_document_revision_number"),
        CheckConstraint(
            "source_type IN ('text', 'media')",
            name="ck_scr_document_revision_source_type",
        ),
        CheckConstraint(
            "(source_type = 'text' AND source_media_version_id IS NULL) OR "
            "(source_type = 'media' AND source_media_version_id IS NOT NULL)",
            name="ck_scr_document_revision_source_reference",
        ),
        CheckConstraint(
            "analysis_status IN ('deterministic', 'ai_candidate_required', 'rejected')",
            name="ck_scr_document_revision_analysis_status",
        ),
        CheckConstraint("codepoint_count >= 1", name="ck_scr_document_revision_codepoints"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_document_revision_id_workspace"),
        UniqueConstraint(
            "document_id",
            "version_no",
            name="uq_scr_document_revision_number",
        ),
        Index("ix_scr_document_revision_raw_hash", "raw_hash"),
        Index(
            "ix_scr_document_revision_document_created",
            "document_id",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    document_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    version_no: Mapped[int] = mapped_column(Integer)
    source_type: Mapped[str] = mapped_column(String(20))
    source_media_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    raw_text: Mapped[str] = mapped_column(Text)
    raw_hash: Mapped[str] = mapped_column(String(64))
    normalized_text: Mapped[str] = mapped_column(Text)
    normalized_hash: Mapped[str] = mapped_column(String(64))
    normalizer_version: Mapped[str] = mapped_column(String(80))
    normalization_map: Mapped[dict[str, Any]] = mapped_column(JSONB)
    codepoint_count: Mapped[int] = mapped_column(Integer)
    analysis_status: Mapped[str] = mapped_column(String(30))
    analyzer_version: Mapped[str] = mapped_column(String(80))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class NarrativeBlock(Base):
    __tablename__ = "scr_narrative_blocks"
    __table_args__ = (
        ForeignKeyConstraint(
            ["document_revision_id", "workspace_id"],
            ["scr_document_revisions.id", "scr_document_revisions.workspace_id"],
            name="fk_scr_narrative_block_revision_workspace",
            ondelete="CASCADE",
        ),
        CheckConstraint("position >= 1", name="ck_scr_narrative_block_position"),
        CheckConstraint(
            "kind IN ('preamble', 'episode_marker', 'scene_heading', 'dialogue', "
            "'narration', 'action', 'separator')",
            name="ck_scr_narrative_block_kind",
        ),
        CheckConstraint("source_start >= 0", name="ck_scr_narrative_block_source_start"),
        CheckConstraint("source_end > source_start", name="ck_scr_narrative_block_source_range"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_narrative_block_id_workspace"),
        UniqueConstraint(
            "document_revision_id",
            "position",
            name="uq_scr_narrative_block_revision_position",
        ),
        Index(
            "ix_scr_narrative_block_revision_range",
            "document_revision_id",
            "source_start",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    document_revision_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    position: Mapped[int] = mapped_column(Integer)
    kind: Mapped[str] = mapped_column(String(30))
    source_start: Mapped[int] = mapped_column(Integer)
    source_end: Mapped[int] = mapped_column(Integer)
    text_hash: Mapped[str] = mapped_column(String(64))
    block_metadata: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class FormatIssue(Base):
    __tablename__ = "scr_format_issues"
    __table_args__ = (
        ForeignKeyConstraint(
            ["document_revision_id", "workspace_id"],
            ["scr_document_revisions.id", "scr_document_revisions.workspace_id"],
            name="fk_scr_format_issue_revision_workspace",
            ondelete="CASCADE",
        ),
        CheckConstraint("position >= 1", name="ck_scr_format_issue_position"),
        CheckConstraint(
            "severity IN ('warning', 'blocking')",
            name="ck_scr_format_issue_severity",
        ),
        CheckConstraint("source_start >= 0", name="ck_scr_format_issue_source_start"),
        CheckConstraint("source_end > source_start", name="ck_scr_format_issue_source_range"),
        CheckConstraint("line_number >= 1", name="ck_scr_format_issue_line"),
        CheckConstraint("column_number >= 1", name="ck_scr_format_issue_column"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_format_issue_id_workspace"),
        UniqueConstraint(
            "document_revision_id",
            "position",
            name="uq_scr_format_issue_revision_position",
        ),
        Index(
            "ix_scr_format_issue_revision_severity",
            "document_revision_id",
            "severity",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    document_revision_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    position: Mapped[int] = mapped_column(Integer)
    code: Mapped[str] = mapped_column(String(80))
    severity: Mapped[str] = mapped_column(String(20))
    source_start: Mapped[int] = mapped_column(Integer)
    source_end: Mapped[int] = mapped_column(Integer)
    line_number: Mapped[int] = mapped_column(Integer)
    column_number: Mapped[int] = mapped_column(Integer)
    next_action: Mapped[str] = mapped_column(String(100))
    issue_details: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class EpisodePlan(Base):
    __tablename__ = "scr_episode_plans"
    __table_args__ = (
        ForeignKeyConstraint(
            ["document_revision_id", "workspace_id"],
            ["scr_document_revisions.id", "scr_document_revisions.workspace_id"],
            name="fk_scr_episode_plan_revision_workspace",
        ),
        ForeignKeyConstraint(
            ["project_id", "workspace_id"],
            ["prj_projects.id", "prj_projects.workspace_id"],
            name="fk_scr_episode_plan_project_workspace",
        ),
        ForeignKeyConstraint(
            ["planning_task_id", "workspace_id"],
            ["prod_tasks.id", "prod_tasks.workspace_id"],
            name="fk_scr_episode_plan_task_workspace",
        ),
        CheckConstraint(
            "strategy IN ('explicit_markers', 'target_duration_ai')",
            name="ck_scr_episode_plan_strategy",
        ),
        CheckConstraint(
            "status IN ('draft', 'review_ready', 'confirmed', 'materialized', 'superseded')",
            name="ck_scr_episode_plan_status",
        ),
        CheckConstraint("revision >= 1", name="ck_scr_episode_plan_revision"),
        CheckConstraint("target_duration_ms >= 1000", name="ck_scr_episode_plan_duration"),
        CheckConstraint(
            "requested_episode_count IS NULL OR "
            "requested_episode_count >= 1",
            name="ck_scr_episode_plan_requested_count",
        ),
        CheckConstraint(
            "total_estimated_duration_ms >= 0",
            name="ck_scr_episode_plan_total_duration",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_scr_episode_plan_id_workspace"),
        UniqueConstraint(
            "project_id",
            "idempotency_key",
            name="uq_scr_episode_plan_project_idempotency",
        ),
        Index(
            "ix_scr_episode_plan_revision_status_created",
            "document_revision_id",
            "status",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    document_revision_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    strategy: Mapped[str] = mapped_column(String(40))
    status: Mapped[str] = mapped_column(String(30), default="draft")
    target_duration_ms: Mapped[int] = mapped_column(Integer)
    requested_episode_count: Mapped[int | None] = mapped_column(Integer, nullable=True)
    total_estimated_duration_ms: Mapped[int] = mapped_column(Integer, default=0)
    input_hash: Mapped[str] = mapped_column(String(64))
    planning_engine_version: Mapped[str] = mapped_column(String(80))
    model_name: Mapped[str | None] = mapped_column(String(160), nullable=True)
    prompt_version: Mapped[str | None] = mapped_column(String(80), nullable=True)
    schema_version: Mapped[str] = mapped_column(String(80))
    planning_task_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    planning_error_code: Mapped[str | None] = mapped_column(String(80), nullable=True)
    command_receipts: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    confirmed_by: Mapped[UUID | None] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=True
    )
    confirmed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class EpisodeProposal(Base):
    __tablename__ = "scr_episode_proposals"
    __table_args__ = (
        ForeignKeyConstraint(
            ["plan_id", "workspace_id"],
            ["scr_episode_plans.id", "scr_episode_plans.workspace_id"],
            name="fk_scr_episode_proposal_plan_workspace",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ["start_block_id", "workspace_id"],
            ["scr_narrative_blocks.id", "scr_narrative_blocks.workspace_id"],
            name="fk_scr_episode_proposal_start_block_workspace",
        ),
        ForeignKeyConstraint(
            ["end_block_id", "workspace_id"],
            ["scr_narrative_blocks.id", "scr_narrative_blocks.workspace_id"],
            name="fk_scr_episode_proposal_end_block_workspace",
        ),
        CheckConstraint("position >= 1", name="ck_scr_episode_proposal_position"),
        CheckConstraint(
            "start_block_position >= 1",
            name="ck_scr_episode_proposal_start_block_position",
        ),
        CheckConstraint(
            "end_block_position >= start_block_position",
            name="ck_scr_episode_proposal_block_range",
        ),
        CheckConstraint("source_start >= 0", name="ck_scr_episode_proposal_source_start"),
        CheckConstraint("source_end > source_start", name="ck_scr_episode_proposal_source_range"),
        CheckConstraint(
            "estimated_duration_ms >= 1000",
            name="ck_scr_episode_proposal_duration",
        ),
        CheckConstraint(
            "confidence >= 0 AND confidence <= 1",
            name="ck_scr_episode_proposal_confidence",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_scr_episode_proposal_id_workspace"),
        UniqueConstraint("plan_id", "position", name="uq_scr_episode_proposal_plan_position"),
        Index(
            "ix_scr_episode_proposal_plan_range",
            "plan_id",
            "source_start",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    plan_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    position: Mapped[int] = mapped_column(Integer)
    title: Mapped[str] = mapped_column(String(120))
    start_block_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    end_block_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    start_block_position: Mapped[int] = mapped_column(Integer)
    end_block_position: Mapped[int] = mapped_column(Integer)
    source_start: Mapped[int] = mapped_column(Integer)
    source_end: Mapped[int] = mapped_column(Integer)
    content_hash: Mapped[str] = mapped_column(String(64))
    estimated_duration_ms: Mapped[int] = mapped_column(Integer)
    reason: Mapped[str] = mapped_column(Text)
    confidence: Mapped[Decimal] = mapped_column(Numeric(5, 4))
    boundary_evidence: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    is_locked: Mapped[bool] = mapped_column(Boolean, default=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class ImportCommit(Base):
    __tablename__ = "scr_import_commits"
    __table_args__ = (
        ForeignKeyConstraint(
            ["plan_id", "workspace_id"],
            ["scr_episode_plans.id", "scr_episode_plans.workspace_id"],
            name="fk_scr_import_commit_plan_workspace",
        ),
        ForeignKeyConstraint(
            ["project_id", "workspace_id"],
            ["prj_projects.id", "prj_projects.workspace_id"],
            name="fk_scr_import_commit_project_workspace",
        ),
        CheckConstraint("mode IN ('append_new')", name="ck_scr_import_commit_mode"),
        CheckConstraint(
            "status IN ('pending', 'materializing', 'materialized', 'publishing', "
            "'published', 'conflict', 'failed')",
            name="ck_scr_import_commit_status",
        ),
        CheckConstraint("revision >= 1", name="ck_scr_import_commit_revision"),
        CheckConstraint(
            "expected_project_revision >= 1",
            name="ck_scr_import_commit_project_revision",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_scr_import_commit_id_workspace"),
        UniqueConstraint(
            "workspace_id",
            "idempotency_key",
            name="uq_scr_import_commit_workspace_idempotency",
        ),
        UniqueConstraint(
            "workspace_id",
            "publish_idempotency_key",
            name="uq_scr_import_commit_publish_idempotency",
        ),
        Index(
            "ix_scr_import_commit_plan_created",
            "plan_id",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    plan_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    mode: Mapped[str] = mapped_column(String(30))
    status: Mapped[str] = mapped_column(String(30), default="pending")
    input_hash: Mapped[str] = mapped_column(String(64))
    expected_project_revision: Mapped[int] = mapped_column(Integer)
    expected_active_order_hash: Mapped[str] = mapped_column(String(64))
    result_snapshot: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    publish_input_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    publish_idempotency_key: Mapped[str | None] = mapped_column(String(200), nullable=True)
    error_code: Mapped[str | None] = mapped_column(String(80), nullable=True)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class ScriptSource(Base):
    __tablename__ = "scr_script_sources"
    __table_args__ = (
        ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_scr_source_episode_workspace",
        ),
        CheckConstraint("input_type IN ('text', 'media')", name="ck_scr_source_input_type"),
        CheckConstraint("status IN ('active', 'archived')", name="ck_scr_source_status"),
        CheckConstraint("revision >= 1", name="ck_scr_source_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_source_id_workspace"),
        UniqueConstraint(
            "episode_id",
            "idempotency_key",
            name="uq_scr_source_episode_idempotency",
        ),
        Index("ix_scr_source_episode_status", "episode_id", "status"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    input_type: Mapped[str] = mapped_column(String(20))
    title: Mapped[str] = mapped_column(String(120))
    source_media_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    rights_declaration: Mapped[str] = mapped_column(Text)
    status: Mapped[str] = mapped_column(String(20), default="active")
    revision: Mapped[int] = mapped_column(Integer, default=1)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    archived_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    archived_by: Mapped[UUID | None] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=True
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class ScriptVersion(Base):
    __tablename__ = "scr_script_versions"
    __table_args__ = (
        ForeignKeyConstraint(
            ["source_id", "workspace_id"],
            ["scr_script_sources.id", "scr_script_sources.workspace_id"],
            name="fk_scr_version_source_workspace",
        ),
        CheckConstraint("version_no >= 1", name="ck_scr_version_number"),
        CheckConstraint("status IN ('draft', 'published')", name="ck_scr_version_status"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_version_id_workspace"),
        UniqueConstraint("source_id", "version_no", name="uq_scr_version_number"),
        Index("ix_scr_version_content_hash", "content_hash"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    source_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    version_no: Mapped[int] = mapped_column(Integer)
    status: Mapped[str] = mapped_column(String(20), default="draft")
    body: Mapped[str] = mapped_column(Text)
    content_hash: Mapped[str] = mapped_column(String(64))
    structure_summary: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class AdaptationRun(Base):
    __tablename__ = "scr_adaptation_runs"
    __table_args__ = (
        ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_scr_adaptation_episode_workspace",
        ),
        ForeignKeyConstraint(
            ["source_id", "workspace_id"],
            ["scr_script_sources.id", "scr_script_sources.workspace_id"],
            name="fk_scr_adaptation_source_workspace",
        ),
        ForeignKeyConstraint(
            ["input_script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_adaptation_input_workspace",
        ),
        ForeignKeyConstraint(
            ["published_script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_adaptation_published_workspace",
        ),
        ForeignKeyConstraint(
            ["task_id", "workspace_id"],
            ["prod_tasks.id", "prod_tasks.workspace_id"],
            name="fk_scr_adaptation_task_workspace",
        ),
        CheckConstraint(
            "status IN ('queued', 'running', 'succeeded', 'published', "
            "'failed', 'cancelled', 'unknown')",
            name="ck_scr_adaptation_status",
        ),
        CheckConstraint(
            "target_duration_ms >= 15000 AND target_duration_ms <= 600000",
            name="ck_scr_adaptation_target_duration",
        ),
        CheckConstraint(
            "pacing IN ('slow', 'balanced', 'fast')",
            name="ck_scr_adaptation_pacing",
        ),
        CheckConstraint("revision >= 1", name="ck_scr_adaptation_revision"),
        CheckConstraint(
            "estimated_duration_ms IS NULL OR "
            "(estimated_duration_ms >= 1000 AND estimated_duration_ms <= 600000)",
            name="ck_scr_adaptation_estimated_duration",
        ),
        CheckConstraint(
            "(candidate_body IS NULL) = (candidate_hash IS NULL)",
            name="ck_scr_adaptation_candidate_pair",
        ),
        CheckConstraint(
            "(draft_body IS NULL) = (draft_hash IS NULL)",
            name="ck_scr_adaptation_draft_pair",
        ),
        CheckConstraint(
            "status != 'published' OR published_script_version_id IS NOT NULL",
            name="ck_scr_adaptation_published_version",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_scr_adaptation_id_workspace"),
        UniqueConstraint("task_id", name="uq_scr_adaptation_task"),
        UniqueConstraint(
            "episode_id",
            "idempotency_key",
            name="uq_scr_adaptation_episode_idempotency",
        ),
        UniqueConstraint(
            "workspace_id",
            "publish_idempotency_key",
            name="uq_scr_adaptation_publish_idempotency",
        ),
        Index(
            "ix_scr_adaptation_episode_created",
            "episode_id",
            "created_at",
        ),
        Index(
            "ix_scr_adaptation_workspace_status_created",
            "workspace_id",
            "status",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    source_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    input_script_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    input_hash: Mapped[str] = mapped_column(String(64))
    target_duration_ms: Mapped[int] = mapped_column(Integer)
    core_plot_points: Mapped[list[str]] = mapped_column(JSONB)
    pacing: Mapped[str] = mapped_column(String(20))
    colloquial_dialogue: Mapped[bool] = mapped_column(Boolean)
    adaptation_engine_version: Mapped[str] = mapped_column(String(80))
    model_name: Mapped[str] = mapped_column(String(160))
    prompt_version: Mapped[str] = mapped_column(String(80))
    schema_version: Mapped[str] = mapped_column(String(80))
    task_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    status: Mapped[str] = mapped_column(String(30), default="queued")
    candidate_body: Mapped[str | None] = mapped_column(Text, nullable=True)
    candidate_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    draft_body: Mapped[str | None] = mapped_column(Text, nullable=True)
    draft_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    change_summary: Mapped[str | None] = mapped_column(Text, nullable=True)
    estimated_duration_ms: Mapped[int | None] = mapped_column(Integer, nullable=True)
    error_code: Mapped[str | None] = mapped_column(String(80), nullable=True)
    published_script_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    publish_idempotency_key: Mapped[str | None] = mapped_column(String(200), nullable=True)
    publish_command_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    publish_result_snapshot: Mapped[dict[str, object]] = mapped_column(JSONB, default=dict)
    cancel_idempotency_key: Mapped[str | None] = mapped_column(String(200), nullable=True)
    cancel_command_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class EpisodeSegmentOrigin(Base):
    __tablename__ = "scr_episode_segment_origins"
    __table_args__ = (
        ForeignKeyConstraint(
            ["import_commit_id", "workspace_id"],
            ["scr_import_commits.id", "scr_import_commits.workspace_id"],
            name="fk_scr_segment_origin_commit_workspace",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ["proposal_id", "workspace_id"],
            ["scr_episode_proposals.id", "scr_episode_proposals.workspace_id"],
            name="fk_scr_segment_origin_proposal_workspace",
        ),
        ForeignKeyConstraint(
            ["document_revision_id", "workspace_id"],
            ["scr_document_revisions.id", "scr_document_revisions.workspace_id"],
            name="fk_scr_segment_origin_revision_workspace",
        ),
        ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_scr_segment_origin_episode_workspace",
        ),
        ForeignKeyConstraint(
            ["source_id", "workspace_id"],
            ["scr_script_sources.id", "scr_script_sources.workspace_id"],
            name="fk_scr_segment_origin_source_workspace",
        ),
        ForeignKeyConstraint(
            ["draft_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_segment_origin_draft_workspace",
        ),
        ForeignKeyConstraint(
            ["published_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_segment_origin_published_workspace",
        ),
        CheckConstraint("position >= 1", name="ck_scr_segment_origin_position"),
        CheckConstraint("source_start >= 0", name="ck_scr_segment_origin_source_start"),
        CheckConstraint("source_end > source_start", name="ck_scr_segment_origin_source_range"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_segment_origin_id_workspace"),
        UniqueConstraint(
            "import_commit_id",
            "position",
            name="uq_scr_segment_origin_commit_position",
        ),
        UniqueConstraint(
            "import_commit_id",
            "episode_id",
            name="uq_scr_segment_origin_commit_episode",
        ),
        Index(
            "ix_scr_segment_origin_revision_range",
            "document_revision_id",
            "source_start",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    import_commit_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    proposal_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    document_revision_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    source_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    draft_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    published_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    position: Mapped[int] = mapped_column(Integer)
    source_start: Mapped[int] = mapped_column(Integer)
    source_end: Mapped[int] = mapped_column(Integer)
    source_hash: Mapped[str] = mapped_column(String(64))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class Scene(Base):
    __tablename__ = "scr_scenes"
    __table_args__ = (
        ForeignKeyConstraint(
            ["script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_scene_version_workspace",
            ondelete="CASCADE",
        ),
        CheckConstraint("position >= 1", name="ck_scr_scene_position"),
        CheckConstraint("source_start >= 0", name="ck_scr_scene_source_start"),
        CheckConstraint("source_end > source_start", name="ck_scr_scene_source_range"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_scene_id_workspace"),
        UniqueConstraint(
            "script_version_id",
            "position",
            name="uq_scr_scene_version_position",
        ),
        Index("ix_scr_scene_version_range", "script_version_id", "source_start"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    script_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    position: Mapped[int] = mapped_column(Integer)
    heading: Mapped[str] = mapped_column(String(200))
    location: Mapped[str] = mapped_column(String(200))
    time_of_day: Mapped[str] = mapped_column(String(100))
    summary: Mapped[str] = mapped_column(Text)
    semantic_context: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    source_start: Mapped[int] = mapped_column(Integer)
    source_end: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class Dialogue(Base):
    __tablename__ = "scr_dialogues"
    __table_args__ = (
        ForeignKeyConstraint(
            ["scene_id", "workspace_id"],
            ["scr_scenes.id", "scr_scenes.workspace_id"],
            name="fk_scr_dialogue_scene_workspace",
            ondelete="CASCADE",
        ),
        CheckConstraint("position >= 1", name="ck_scr_dialogue_position"),
        CheckConstraint(
            "dialogue_kind IN ('spoken', 'narration', 'internal', 'voice_over')",
            name="ck_scr_dialogue_kind",
        ),
        CheckConstraint("source_start >= 0", name="ck_scr_dialogue_source_start"),
        CheckConstraint(
            "source_end > source_start",
            name="ck_scr_dialogue_source_range",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_scr_dialogue_id_workspace"),
        UniqueConstraint("scene_id", "position", name="uq_scr_dialogue_scene_position"),
        Index("ix_scr_dialogue_scene_range", "scene_id", "source_start"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    scene_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    position: Mapped[int] = mapped_column(Integer)
    speaker_candidate: Mapped[str] = mapped_column(String(200))
    dialogue_kind: Mapped[str] = mapped_column(String(30))
    text: Mapped[str] = mapped_column(Text)
    performance_note: Mapped[str | None] = mapped_column(Text, nullable=True)
    source_start: Mapped[int] = mapped_column(Integer)
    source_end: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class ExtractionBatch(Base):
    __tablename__ = "scr_extraction_batches"
    __table_args__ = (
        ForeignKeyConstraint(
            ["script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_batch_version_workspace",
        ),
        ForeignKeyConstraint(
            ["confirmed_script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_batch_confirmed_version_workspace",
        ),
        ForeignKeyConstraint(
            ["task_id", "workspace_id"],
            ["prod_tasks.id", "prod_tasks.workspace_id"],
            name="fk_scr_batch_task_workspace",
        ),
        CheckConstraint("scope = 'full'", name="ck_scr_batch_scope"),
        CheckConstraint(
            "status IN ('queued', 'running', 'waiting_provider', 'succeeded', "
            "'failed', 'cancelled', 'unknown')",
            name="ck_scr_batch_status",
        ),
        CheckConstraint("candidate_count >= 0", name="ck_scr_batch_candidate_count"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_batch_id_workspace"),
        UniqueConstraint(
            "script_version_id",
            "idempotency_key",
            name="uq_scr_batch_version_idempotency",
        ),
        UniqueConstraint("task_id", name="uq_scr_batch_task"),
        Index(
            "ix_scr_batch_workspace_status_created",
            "workspace_id",
            "status",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    script_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    task_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    scope: Mapped[str] = mapped_column(String(20), default="full")
    extractor_version: Mapped[str] = mapped_column(String(80))
    input_hash: Mapped[str] = mapped_column(String(64))
    status: Mapped[str] = mapped_column(String(30), default="queued")
    confirmed_script_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    result_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    candidate_count: Mapped[int] = mapped_column(Integer, default=0)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class ExtractionCandidate(Base):
    __tablename__ = "scr_extraction_candidates"
    __table_args__ = (
        ForeignKeyConstraint(
            ["batch_id", "workspace_id"],
            ["scr_extraction_batches.id", "scr_extraction_batches.workspace_id"],
            name="fk_scr_candidate_batch_workspace",
        ),
        CheckConstraint(
            "kind IN ('scene', 'dialogue', 'asset', 'shot', 'continuity')",
            name="ck_scr_candidate_kind",
        ),
        CheckConstraint(
            "status IN ('pending', 'accepted', 'linked', 'merged', 'ignored')",
            name="ck_scr_candidate_status",
        ),
        CheckConstraint("source_start >= 0", name="ck_scr_candidate_source_start"),
        CheckConstraint("source_end > source_start", name="ck_scr_candidate_source_range"),
        CheckConstraint("revision >= 1", name="ck_scr_candidate_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_candidate_id_workspace"),
        UniqueConstraint("batch_id", "candidate_key", name="uq_scr_candidate_batch_key"),
        Index(
            "ix_scr_candidate_batch_status_range",
            "batch_id",
            "status",
            "source_start",
            "source_end",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    batch_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    candidate_key: Mapped[str] = mapped_column(String(100))
    kind: Mapped[str] = mapped_column(String(30))
    source_start: Mapped[int] = mapped_column(Integer)
    source_end: Mapped[int] = mapped_column(Integer)
    proposal: Mapped[dict[str, Any]] = mapped_column(JSONB)
    confidence_note: Mapped[str | None] = mapped_column(Text, nullable=True)
    required: Mapped[bool] = mapped_column(Boolean)
    status: Mapped[str] = mapped_column(String(30), default="pending")
    revision: Mapped[int] = mapped_column(Integer, default=1)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class CandidateDecision(Base):
    __tablename__ = "scr_candidate_decisions"
    __table_args__ = (
        ForeignKeyConstraint(
            ["candidate_id", "workspace_id"],
            ["scr_extraction_candidates.id", "scr_extraction_candidates.workspace_id"],
            name="fk_scr_decision_candidate_workspace",
        ),
        CheckConstraint("sequence >= 1", name="ck_scr_decision_sequence"),
        CheckConstraint(
            "action IN ('accept_new', 'accept_with_changes', 'link_existing', "
            "'merge_into', 'ignore')",
            name="ck_scr_decision_action",
        ),
        UniqueConstraint("candidate_id", "sequence", name="uq_scr_decision_candidate_sequence"),
        UniqueConstraint("candidate_id", "decision_key", name="uq_scr_decision_candidate_key"),
        Index("ix_scr_decision_candidate_created", "candidate_id", "created_at"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    candidate_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    sequence: Mapped[int] = mapped_column(Integer)
    decision_key: Mapped[str] = mapped_column(String(200))
    action: Mapped[str] = mapped_column(String(40))
    payload: Mapped[dict[str, Any]] = mapped_column(JSONB)
    downstream_type: Mapped[str | None] = mapped_column(String(40), nullable=True)
    downstream_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    actor_id: Mapped[UUID] = mapped_column(Uuid, ForeignKey("idn_user_accounts.id"), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
