from datetime import UTC, datetime
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
        CheckConstraint(
            "source_end > source_start", name="ck_scr_candidate_source_range"
        ),
        CheckConstraint("revision >= 1", name="ck_scr_candidate_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_candidate_id_workspace"),
        UniqueConstraint(
            "batch_id", "candidate_key", name="uq_scr_candidate_batch_key"
        ),
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
        UniqueConstraint(
            "candidate_id", "sequence", name="uq_scr_decision_candidate_sequence"
        ),
        UniqueConstraint(
            "candidate_id", "decision_key", name="uq_scr_decision_candidate_key"
        ),
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
    actor_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
