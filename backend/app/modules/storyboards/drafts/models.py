from datetime import UTC, datetime
from typing import Any
from uuid import UUID

from sqlalchemy import (
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
from sqlalchemy.dialects.postgresql import ARRAY, JSONB
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class StoryboardDraftBatch(Base):
    __tablename__ = "sbd_draft_batches"
    __table_args__ = (
        ForeignKeyConstraint(
            ("project_id", "workspace_id"),
            ("prj_projects.id", "prj_projects.workspace_id"),
            name="fk_sbd_draft_batch_project",
        ),
        ForeignKeyConstraint(
            ("episode_id", "workspace_id"),
            ("prj_episodes.id", "prj_episodes.workspace_id"),
            name="fk_sbd_draft_batch_episode",
        ),
        ForeignKeyConstraint(
            ("input_script_version_id", "workspace_id"),
            ("scr_script_versions.id", "scr_script_versions.workspace_id"),
            name="fk_sbd_draft_batch_script",
        ),
        ForeignKeyConstraint(
            (
                "narrative_structure_id",
                "input_script_version_id",
                "episode_id",
                "workspace_id",
            ),
            (
                "scr_narrative_structures.id",
                "scr_narrative_structures.script_version_id",
                "scr_narrative_structures.episode_id",
                "scr_narrative_structures.workspace_id",
            ),
            name="fk_sbd_draft_batch_narrative",
        ),
        ForeignKeyConstraint(
            ("task_id", "workspace_id"),
            ("prod_tasks.id", "prod_tasks.workspace_id"),
            name="fk_sbd_draft_batch_task",
        ),
        CheckConstraint(
            "status IN ('queued', 'running', 'needs_review', 'approved', "
            "'applied', 'failed', 'unknown', 'cancelled')",
            name="ck_sbd_draft_batch_status",
        ),
        CheckConstraint("narrative_revision >= 1", name="ck_sbd_draft_narrative_revision"),
        CheckConstraint("revision >= 1", name="ck_sbd_draft_batch_revision"),
        CheckConstraint(
            "target_duration_ms >= 1000 AND target_duration_ms <= 7200000",
            name="ck_sbd_draft_batch_duration",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_sbd_draft_batch_workspace"),
        UniqueConstraint(
            "id",
            "episode_id",
            "workspace_id",
            name="uq_sbd_draft_batch_scope",
        ),
        UniqueConstraint("task_id", name="uq_sbd_draft_batch_task"),
        UniqueConstraint(
            "episode_id",
            "idempotency_key",
            name="uq_sbd_draft_batch_idempotency",
        ),
        UniqueConstraint(
            "workspace_id",
            "approve_idempotency_key",
            name="uq_sbd_draft_approve_idempotency",
        ),
        UniqueConstraint(
            "workspace_id",
            "apply_idempotency_key",
            name="uq_sbd_draft_apply_idempotency",
        ),
        Index("ix_sbd_draft_episode_created", "episode_id", "created_at"),
        Index("ix_sbd_draft_workspace_status", "workspace_id", "status"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    input_script_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    narrative_structure_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    narrative_revision: Mapped[int] = mapped_column(Integer)
    narrative_dependency_hash: Mapped[str] = mapped_column(String(64))
    input_hash: Mapped[str] = mapped_column(String(64))
    target_duration_ms: Mapped[int] = mapped_column(Integer)
    aspect_ratio: Mapped[str] = mapped_column(String(10))
    visual_style: Mapped[str | None] = mapped_column(String(200), nullable=True)
    engine_version: Mapped[str] = mapped_column(String(80))
    model_name: Mapped[str] = mapped_column(String(160))
    prompt_version: Mapped[str] = mapped_column(String(80))
    schema_version: Mapped[str] = mapped_column(String(80))
    base_order_hash: Mapped[str] = mapped_column(String(64))
    base_shot_hash: Mapped[str] = mapped_column(String(64))
    base_shots: Mapped[list[dict[str, object]]] = mapped_column(JSONB)
    task_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    status: Mapped[str] = mapped_column(String(30), default="queued")
    provider_result_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    error_code: Mapped[str | None] = mapped_column(String(80), nullable=True)
    approve_idempotency_key: Mapped[str | None] = mapped_column(String(200), nullable=True)
    approve_command_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    apply_idempotency_key: Mapped[str | None] = mapped_column(String(200), nullable=True)
    apply_command_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    apply_result: Mapped[dict[str, object]] = mapped_column(JSONB, default=dict)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class DraftInputUnit(Base):
    __tablename__ = "sbd_draft_input_units"
    __table_args__ = (
        ForeignKeyConstraint(
            ("batch_id", "episode_id", "workspace_id"),
            (
                "sbd_draft_batches.id",
                "sbd_draft_batches.episode_id",
                "sbd_draft_batches.workspace_id",
            ),
            name="fk_sbd_draft_unit_batch",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ("unit_version_id", "narrative_unit_id", "episode_id", "workspace_id"),
            (
                "scr_narrative_unit_versions.id",
                "scr_narrative_unit_versions.unit_id",
                "scr_narrative_unit_versions.episode_id",
                "scr_narrative_unit_versions.workspace_id",
            ),
            name="fk_sbd_draft_unit_version",
        ),
        CheckConstraint("position >= 1", name="ck_sbd_draft_unit_position"),
        UniqueConstraint(
            "batch_id",
            "unit_version_id",
            "workspace_id",
            name="uq_sbd_draft_unit_input",
        ),
        UniqueConstraint(
            "batch_id",
            "position",
            name="uq_sbd_draft_unit_position",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    batch_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    narrative_unit_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    unit_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    position: Mapped[int] = mapped_column(Integer)
    kind: Mapped[str] = mapped_column(String(30))
    exact_text: Mapped[str] = mapped_column(Text)
    text_hash: Mapped[str] = mapped_column(String(64))
    required_for_coverage: Mapped[bool]
    source_scene_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    source_dialogue_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)


class DraftInputAsset(Base):
    __tablename__ = "sbd_draft_input_assets"
    __table_args__ = (
        ForeignKeyConstraint(
            ("batch_id", "workspace_id"),
            ("sbd_draft_batches.id", "sbd_draft_batches.workspace_id"),
            name="fk_sbd_draft_asset_batch",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ("asset_version_id", "asset_state_id", "asset_id", "workspace_id"),
            (
                "ast_asset_versions.id",
                "ast_asset_versions.asset_state_id",
                "ast_asset_versions.asset_id",
                "ast_asset_versions.workspace_id",
            ),
            name="fk_sbd_draft_asset_version",
        ),
        CheckConstraint("position >= 1", name="ck_sbd_draft_asset_position"),
        CheckConstraint("state_revision >= 1", name="ck_sbd_draft_asset_revision"),
        UniqueConstraint(
            "batch_id",
            "asset_version_id",
            "workspace_id",
            name="uq_sbd_draft_asset_input",
        ),
        UniqueConstraint(
            "batch_id",
            "asset_state_id",
            name="uq_sbd_draft_asset_state",
        ),
        UniqueConstraint(
            "batch_id",
            "position",
            name="uq_sbd_draft_asset_position",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    batch_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    asset_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    asset_state_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    asset_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    position: Mapped[int] = mapped_column(Integer)
    kind: Mapped[str] = mapped_column(String(30))
    name: Mapped[str] = mapped_column(String(200))
    state_label: Mapped[str] = mapped_column(String(120))
    state_revision: Mapped[int] = mapped_column(Integer)
    readiness_hash: Mapped[str] = mapped_column(String(64))


class DraftShot(Base):
    __tablename__ = "sbd_draft_shots"
    __table_args__ = (
        ForeignKeyConstraint(
            ("batch_id", "workspace_id"),
            ("sbd_draft_batches.id", "sbd_draft_batches.workspace_id"),
            name="fk_sbd_draft_shot_batch",
            ondelete="CASCADE",
        ),
        CheckConstraint("position >= 1 AND position <= 120", name="ck_sbd_draft_shot_position"),
        UniqueConstraint("id", "workspace_id", name="uq_sbd_draft_shot_workspace"),
        UniqueConstraint(
            "id",
            "batch_id",
            "workspace_id",
            name="uq_sbd_draft_shot_scope",
        ),
        UniqueConstraint("batch_id", "proposal_key", name="uq_sbd_draft_shot_key"),
        UniqueConstraint("batch_id", "position", name="uq_sbd_draft_shot_position"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    batch_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    proposal_key: Mapped[str] = mapped_column(String(120))
    position: Mapped[int] = mapped_column(Integer)
    title: Mapped[str] = mapped_column(String(200))
    spec: Mapped[dict[str, Any]] = mapped_column(JSONB)
    content_hash: Mapped[str] = mapped_column(String(64))
    risk_codes: Mapped[list[str]] = mapped_column(ARRAY(String(80)), default=list)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class DraftShotUnit(Base):
    __tablename__ = "sbd_draft_shot_units"
    __table_args__ = (
        ForeignKeyConstraint(
            ("draft_shot_id", "batch_id", "workspace_id"),
            ("sbd_draft_shots.id", "sbd_draft_shots.batch_id", "sbd_draft_shots.workspace_id"),
            name="fk_sbd_draft_shot_unit_shot",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ("batch_id", "unit_version_id", "workspace_id"),
            (
                "sbd_draft_input_units.batch_id",
                "sbd_draft_input_units.unit_version_id",
                "sbd_draft_input_units.workspace_id",
            ),
            name="fk_sbd_draft_shot_unit_input",
        ),
        CheckConstraint("position >= 1", name="ck_sbd_draft_shot_unit_position"),
        UniqueConstraint(
            "draft_shot_id",
            "unit_version_id",
            name="uq_sbd_draft_shot_unit",
        ),
        UniqueConstraint(
            "draft_shot_id",
            "position",
            name="uq_sbd_draft_shot_unit_position",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    batch_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    draft_shot_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    unit_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    position: Mapped[int] = mapped_column(Integer)


class DraftAssetReference(Base):
    __tablename__ = "sbd_draft_asset_refs"
    __table_args__ = (
        ForeignKeyConstraint(
            ("draft_shot_id", "batch_id", "workspace_id"),
            ("sbd_draft_shots.id", "sbd_draft_shots.batch_id", "sbd_draft_shots.workspace_id"),
            name="fk_sbd_draft_ref_shot",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ("batch_id", "asset_version_id", "workspace_id"),
            (
                "sbd_draft_input_assets.batch_id",
                "sbd_draft_input_assets.asset_version_id",
                "sbd_draft_input_assets.workspace_id",
            ),
            name="fk_sbd_draft_ref_input",
        ),
        CheckConstraint(
            "role IN ('location', 'character', 'prop', 'costume', 'visual_style', 'voice')",
            name="ck_sbd_draft_ref_role",
        ),
        UniqueConstraint("draft_shot_id", "slot_key", name="uq_sbd_draft_ref_slot"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    batch_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    draft_shot_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    slot_key: Mapped[str] = mapped_column(String(100))
    role: Mapped[str] = mapped_column(String(30))
    asset_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    subject_key: Mapped[str | None] = mapped_column(String(100), nullable=True)


class DraftDecision(Base):
    __tablename__ = "sbd_draft_decisions"
    __table_args__ = (
        ForeignKeyConstraint(
            ("draft_shot_id", "batch_id", "workspace_id"),
            ("sbd_draft_shots.id", "sbd_draft_shots.batch_id", "sbd_draft_shots.workspace_id"),
            name="fk_sbd_draft_decision_shot",
            ondelete="CASCADE",
        ),
        CheckConstraint(
            "action IN ('accepted', 'modified', 'ignored')",
            name="ck_sbd_draft_decision_action",
        ),
        CheckConstraint("sequence >= 1", name="ck_sbd_draft_decision_sequence"),
        CheckConstraint(
            "(action = 'modified') = (target IS NOT NULL)",
            name="ck_sbd_draft_decision_target",
        ),
        UniqueConstraint("batch_id", "sequence", name="uq_sbd_draft_decision_sequence"),
        UniqueConstraint(
            "workspace_id",
            "idempotency_key",
            name="uq_sbd_draft_decision_idempotency",
        ),
        Index("ix_sbd_draft_decision_shot", "draft_shot_id", "sequence"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    batch_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    draft_shot_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    sequence: Mapped[int] = mapped_column(Integer)
    action: Mapped[str] = mapped_column(String(20))
    target: Mapped[dict[str, Any] | None] = mapped_column(
        JSONB(none_as_null=True),
        nullable=True,
    )
    command_hash: Mapped[str] = mapped_column(String(64))
    idempotency_key: Mapped[str] = mapped_column(String(200))
    actor_id: Mapped[UUID] = mapped_column(Uuid, ForeignKey("idn_user_accounts.id"), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
