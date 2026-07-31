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
    UniqueConstraint,
    Uuid,
    text,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class Shot(Base):
    __tablename__ = "sbd_shots"
    __table_args__ = (
        ForeignKeyConstraint(
            ("episode_id", "workspace_id"),
            ("prj_episodes.id", "prj_episodes.workspace_id"),
            name="fk_sbd_shot_episode_workspace",
        ),
        ForeignKeyConstraint(
            ("source_script_version_id", "workspace_id"),
            ("scr_script_versions.id", "scr_script_versions.workspace_id"),
            name="fk_sbd_shot_script_workspace",
        ),
        ForeignKeyConstraint(
            ("source_scene_id", "workspace_id"),
            ("scr_scenes.id", "scr_scenes.workspace_id"),
            name="fk_sbd_shot_scene_workspace",
        ),
        ForeignKeyConstraint(
            ("source_candidate_id", "workspace_id"),
            ("scr_extraction_candidates.id", "scr_extraction_candidates.workspace_id"),
            name="fk_sbd_shot_candidate_workspace",
        ),
        ForeignKeyConstraint(
            ("current_spec_version_id", "workspace_id"),
            ("sbd_shot_spec_versions.id", "sbd_shot_spec_versions.workspace_id"),
            name="fk_sbd_shot_current_spec_workspace",
            deferrable=True,
            initially="DEFERRED",
            use_alter=True,
        ),
        CheckConstraint("position >= 1", name="ck_sbd_shot_position"),
        CheckConstraint("status IN ('active', 'archived')", name="ck_sbd_shot_status"),
        CheckConstraint("revision >= 1", name="ck_sbd_shot_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_sbd_shot_id_workspace"),
        UniqueConstraint(
            "workspace_id",
            "creation_key",
            name="uq_sbd_shot_workspace_creation_key",
        ),
        Index(
            "uq_sbd_shot_workspace_candidate",
            "workspace_id",
            "source_candidate_id",
            unique=True,
            postgresql_where=text("source_candidate_id IS NOT NULL"),
        ),
        Index(
            "uq_sbd_shot_active_position",
            "episode_id",
            "position",
            unique=True,
            postgresql_where=text("status = 'active'"),
        ),
        Index(
            "ix_sbd_shot_episode_status_position",
            "episode_id",
            "status",
            "position",
        ),
        Index("ix_sbd_shot_script_scene", "source_script_version_id", "source_scene_id"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    position: Mapped[int] = mapped_column(Integer)
    title: Mapped[str] = mapped_column(String(200))
    source_script_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    source_scene_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    source_candidate_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    creation_key: Mapped[str | None] = mapped_column(String(200), nullable=True)
    status: Mapped[str] = mapped_column(String(20), default="active")
    current_spec_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    archived_at: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True
    )
    archived_by: Mapped[UUID | None] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=True
    )
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class ShotSpecVersion(Base):
    __tablename__ = "sbd_shot_spec_versions"
    __table_args__ = (
        ForeignKeyConstraint(
            ("shot_id", "workspace_id"),
            ("sbd_shots.id", "sbd_shots.workspace_id"),
            name="fk_sbd_spec_shot_workspace",
            deferrable=True,
            initially="DEFERRED",
        ),
        CheckConstraint("version_no >= 1", name="ck_sbd_spec_version_number"),
        CheckConstraint("schema_version = 1", name="ck_sbd_spec_schema_version"),
        UniqueConstraint("id", "workspace_id", name="uq_sbd_spec_id_workspace"),
        UniqueConstraint("shot_id", "version_no", name="uq_sbd_spec_version_number"),
        Index("ix_sbd_spec_input_hash", "input_hash"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    shot_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    version_no: Mapped[int] = mapped_column(Integer)
    schema_version: Mapped[int] = mapped_column(Integer, default=1)
    spec: Mapped[dict[str, Any]] = mapped_column(JSONB)
    content_hash: Mapped[str] = mapped_column(String(64))
    input_hash: Mapped[str] = mapped_column(String(64))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now
    )


class AssetReference(Base):
    __tablename__ = "sbd_asset_references"
    __table_args__ = (
        ForeignKeyConstraint(
            ("shot_spec_version_id", "workspace_id"),
            ("sbd_shot_spec_versions.id", "sbd_shot_spec_versions.workspace_id"),
            name="fk_sbd_asset_ref_spec_workspace",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ("asset_version_id", "workspace_id"),
            ("ast_asset_versions.id", "ast_asset_versions.workspace_id"),
            name="fk_sbd_asset_ref_version_workspace",
        ),
        CheckConstraint(
            "role IN ('location', 'character', 'prop', 'costume', "
            "'visual_style', 'voice')",
            name="ck_sbd_asset_ref_role",
        ),
        UniqueConstraint(
            "shot_spec_version_id",
            "slot_key",
            name="uq_sbd_asset_ref_spec_slot",
        ),
        Index("ix_sbd_asset_ref_asset_version", "asset_version_id"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    shot_spec_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    slot_key: Mapped[str] = mapped_column(String(100))
    role: Mapped[str] = mapped_column(String(30))
    asset_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    subject_key: Mapped[str | None] = mapped_column(String(100), nullable=True)
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now
    )
