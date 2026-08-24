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
from sqlalchemy.dialects.postgresql import ARRAY, JSONB
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class NarrativeStructure(Base):
    __tablename__ = "scr_narrative_structures"
    __table_args__ = (
        ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_scr_narrative_structure_episode_workspace",
        ),
        ForeignKeyConstraint(
            ["script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_narrative_structure_script_workspace",
        ),
        CheckConstraint("revision >= 1", name="ck_scr_narrative_structure_revision"),
        UniqueConstraint(
            "id",
            "workspace_id",
            name="uq_scr_narrative_structure_id_workspace",
        ),
        UniqueConstraint(
            "id",
            "script_version_id",
            "episode_id",
            "workspace_id",
            name="uq_scr_narrative_structure_scope",
        ),
        UniqueConstraint(
            "script_version_id",
            name="uq_scr_narrative_structure_script",
        ),
        Index(
            "ix_scr_narrative_structure_episode_created",
            "episode_id",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    script_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    input_hash: Mapped[str] = mapped_column(String(64))
    parser_version: Mapped[str] = mapped_column(String(80))
    structure_hash: Mapped[str] = mapped_column(String(64))
    dependency_hash: Mapped[str] = mapped_column(String(64))
    revision: Mapped[int] = mapped_column(Integer, default=1)
    command_receipts: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class NarrativeUnit(Base):
    __tablename__ = "scr_narrative_units"
    __table_args__ = (
        ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_scr_narrative_unit_episode_workspace",
        ),
        ForeignKeyConstraint(
            ["current_version_id", "workspace_id"],
            ["scr_narrative_unit_versions.id", "scr_narrative_unit_versions.workspace_id"],
            name="fk_scr_narrative_unit_current_workspace",
            deferrable=True,
            initially="DEFERRED",
            use_alter=True,
        ),
        CheckConstraint(
            "kind IN ('scene_heading', 'action', 'dialogue', 'narration')",
            name="ck_scr_narrative_unit_kind",
        ),
        CheckConstraint(
            "status IN ('active', 'retired')",
            name="ck_scr_narrative_unit_status",
        ),
        CheckConstraint("revision >= 1", name="ck_scr_narrative_unit_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_scr_narrative_unit_id_workspace"),
        UniqueConstraint(
            "id",
            "episode_id",
            "workspace_id",
            name="uq_scr_narrative_unit_scope",
        ),
        Index(
            "ix_scr_narrative_unit_episode_status",
            "episode_id",
            "status",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    kind: Mapped[str] = mapped_column(String(30))
    status: Mapped[str] = mapped_column(String(20), default="active")
    current_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class NarrativeUnitVersion(Base):
    __tablename__ = "scr_narrative_unit_versions"
    __table_args__ = (
        ForeignKeyConstraint(
            ["structure_id", "script_version_id", "episode_id", "workspace_id"],
            [
                "scr_narrative_structures.id",
                "scr_narrative_structures.script_version_id",
                "scr_narrative_structures.episode_id",
                "scr_narrative_structures.workspace_id",
            ],
            name="fk_scr_narrative_version_structure_scope",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ["unit_id", "episode_id", "workspace_id"],
            [
                "scr_narrative_units.id",
                "scr_narrative_units.episode_id",
                "scr_narrative_units.workspace_id",
            ],
            name="fk_scr_narrative_version_unit_scope",
        ),
        ForeignKeyConstraint(
            ["script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_narrative_version_script_workspace",
        ),
        ForeignKeyConstraint(
            ["source_scene_id", "workspace_id"],
            ["scr_scenes.id", "scr_scenes.workspace_id"],
            name="fk_scr_narrative_version_scene_workspace",
        ),
        ForeignKeyConstraint(
            ["source_dialogue_id", "workspace_id"],
            ["scr_dialogues.id", "scr_dialogues.workspace_id"],
            name="fk_scr_narrative_version_dialogue_workspace",
        ),
        CheckConstraint("version_no >= 1", name="ck_scr_narrative_version_number"),
        CheckConstraint(
            "structure_revision >= 1",
            name="ck_scr_narrative_version_structure_revision",
        ),
        CheckConstraint("position >= 1", name="ck_scr_narrative_version_position"),
        CheckConstraint("source_start >= 0", name="ck_scr_narrative_version_start"),
        CheckConstraint(
            "source_end > source_start",
            name="ck_scr_narrative_version_range",
        ),
        CheckConstraint(
            "origin IN ('deterministic', 'manual')",
            name="ck_scr_narrative_version_origin",
        ),
        UniqueConstraint(
            "id",
            "workspace_id",
            name="uq_scr_narrative_version_id_workspace",
        ),
        UniqueConstraint(
            "id",
            "unit_id",
            "episode_id",
            "workspace_id",
            name="uq_scr_narrative_version_unit_scope",
        ),
        UniqueConstraint(
            "unit_id",
            "version_no",
            name="uq_scr_narrative_version_number",
        ),
        UniqueConstraint(
            "structure_id",
            "structure_revision",
            "position",
            name="uq_scr_narrative_version_structure_position",
        ),
        UniqueConstraint(
            "structure_id",
            "structure_revision",
            "unit_id",
            name="uq_scr_narrative_version_structure_unit",
        ),
        Index(
            "ix_scr_narrative_version_script_range",
            "script_version_id",
            "source_start",
        ),
        Index(
            "ix_scr_narrative_version_unit_created",
            "unit_id",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    structure_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    script_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    unit_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    version_no: Mapped[int] = mapped_column(Integer)
    structure_revision: Mapped[int] = mapped_column(Integer)
    position: Mapped[int] = mapped_column(Integer)
    source_start: Mapped[int] = mapped_column(Integer)
    source_end: Mapped[int] = mapped_column(Integer)
    exact_text: Mapped[str] = mapped_column(Text)
    text_hash: Mapped[str] = mapped_column(String(64))
    prefix_text: Mapped[str] = mapped_column(String(120))
    suffix_text: Mapped[str] = mapped_column(String(120))
    required_for_coverage: Mapped[bool] = mapped_column(Boolean, default=True)
    payload: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    source_scene_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    source_dialogue_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    origin: Mapped[str] = mapped_column(String(30))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class NarrativeImpactAssessment(Base):
    __tablename__ = "scr_narrative_impacts"
    __table_args__ = (
        ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_scr_narrative_impact_episode_workspace",
        ),
        ForeignKeyConstraint(
            ["previous_script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_narrative_impact_previous_workspace",
        ),
        ForeignKeyConstraint(
            ["current_script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_narrative_impact_current_workspace",
        ),
        CheckConstraint("sequence >= 1", name="ck_scr_narrative_impact_sequence"),
        CheckConstraint(
            "trigger IN ('current_changed', 'structure_corrected')",
            name="ck_scr_narrative_impact_trigger",
        ),
        CheckConstraint(
            "episode_revision >= 1",
            name="ck_scr_narrative_impact_episode_revision",
        ),
        CheckConstraint(
            "previous_unit_count >= 0 AND current_unit_count >= 0",
            name="ck_scr_narrative_impact_unit_counts",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_scr_narrative_impact_id_workspace"),
        UniqueConstraint(
            "episode_id",
            "sequence",
            name="uq_scr_narrative_impact_episode_sequence",
        ),
        Index(
            "ix_scr_narrative_impact_episode_created",
            "episode_id",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    sequence: Mapped[int] = mapped_column(Integer)
    trigger: Mapped[str] = mapped_column(String(30))
    episode_revision: Mapped[int] = mapped_column(Integer)
    previous_script_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    current_script_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    previous_structure_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    current_structure_hash: Mapped[str] = mapped_column(String(64))
    previous_dependency_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    current_dependency_hash: Mapped[str] = mapped_column(String(64))
    previous_unit_count: Mapped[int] = mapped_column(Integer)
    current_unit_count: Mapped[int] = mapped_column(Integer)
    affected_shot_ids: Mapped[list[UUID]] = mapped_column(ARRAY(Uuid()), default=list)
    invalidated_scopes: Mapped[list[str]] = mapped_column(ARRAY(String(40)), default=list)
    impact_hash: Mapped[str] = mapped_column(String(64))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
