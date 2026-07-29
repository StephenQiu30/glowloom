from datetime import UTC, datetime
from decimal import Decimal
from uuid import UUID

from sqlalchemy import (
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
    text,
)
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class Project(Base):
    __tablename__ = "prj_projects"
    __table_args__ = (
        CheckConstraint("status IN ('active', 'archived')", name="ck_prj_project_status"),
        CheckConstraint("revision >= 1", name="ck_prj_project_revision"),
        CheckConstraint("target_duration_ms > 0", name="ck_prj_project_duration"),
        CheckConstraint("budget_limit >= 0", name="ck_prj_project_budget"),
        UniqueConstraint("id", "workspace_id", name="uq_prj_project_id_workspace"),
        Index("ix_prj_project_workspace_status_updated", "workspace_id", "status", "updated_at"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    name: Mapped[str] = mapped_column(String(120))
    description: Mapped[str | None] = mapped_column(Text, nullable=True)
    aspect_ratio: Mapped[str] = mapped_column(String(10))
    language: Mapped[str] = mapped_column(String(35))
    visual_style: Mapped[str | None] = mapped_column(String(200), nullable=True)
    target_duration_ms: Mapped[int] = mapped_column(Integer)
    budget_limit: Mapped[Decimal] = mapped_column(
        Numeric(20, 6), default=Decimal("0.000000")
    )
    currency: Mapped[str] = mapped_column(String(3), default="CNY")
    status: Mapped[str] = mapped_column(String(20), default="active")
    revision: Mapped[int] = mapped_column(Integer, default=1)
    archived_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    archived_by: Mapped[UUID | None] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=True
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class Episode(Base):
    __tablename__ = "prj_episodes"
    __table_args__ = (
        ForeignKeyConstraint(
            ("project_id", "workspace_id"),
            ("prj_projects.id", "prj_projects.workspace_id"),
            name="fk_prj_episode_project_workspace",
        ),
        CheckConstraint("status IN ('active', 'archived')", name="ck_prj_episode_status"),
        CheckConstraint("revision >= 1", name="ck_prj_episode_revision"),
        CheckConstraint("position >= 1", name="ck_prj_episode_position"),
        CheckConstraint("target_duration_ms > 0", name="ck_prj_episode_duration"),
        UniqueConstraint("id", "workspace_id", name="uq_prj_episode_id_workspace"),
        Index(
            "uq_prj_episode_active_position",
            "project_id",
            "position",
            unique=True,
            postgresql_where=text("status = 'active'"),
        ),
        Index("ix_prj_episode_project_status_position", "project_id", "status", "position"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    name: Mapped[str] = mapped_column(String(120))
    position: Mapped[int] = mapped_column(Integer)
    target_duration_ms: Mapped[int] = mapped_column(Integer)
    status: Mapped[str] = mapped_column(String(20), default="active")
    revision: Mapped[int] = mapped_column(Integer, default=1)
    current_script_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    current_timeline_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    archived_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    archived_by: Mapped[UUID | None] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=True
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )
