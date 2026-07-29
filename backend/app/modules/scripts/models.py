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
