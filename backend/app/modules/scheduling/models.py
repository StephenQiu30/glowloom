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


class Schedule(Base):
    __tablename__ = "sys_schedules"
    __table_args__ = (
        CheckConstraint(
            "kind IN ('one_off', 'interval', 'cron')",
            name="ck_sys_schedule_kind",
        ),
        CheckConstraint(
            "status IN ('active', 'paused', 'completed', 'manual_attention')",
            name="ck_sys_schedule_status",
        ),
        CheckConstraint(
            "misfire_policy IN ('skip', 'run_once', 'catch_up')",
            name="ck_sys_schedule_misfire_policy",
        ),
        CheckConstraint("max_catch_up >= 0", name="ck_sys_schedule_max_catch_up"),
        CheckConstraint("failure_count >= 0", name="ck_sys_schedule_failure_count"),
        CheckConstraint("revision >= 1", name="ck_sys_schedule_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_sys_schedule_id_workspace"),
        UniqueConstraint(
            "workspace_id", "schedule_key", name="uq_sys_schedule_workspace_key"
        ),
        Index(
            "ix_sys_schedule_due",
            "status",
            "next_fire_at",
            "next_attempt_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    schedule_key: Mapped[str] = mapped_column(String(200))
    handler_name: Mapped[str] = mapped_column(String(100))
    scope: Mapped[dict[str, Any]] = mapped_column(JSONB)
    payload: Mapped[dict[str, Any]] = mapped_column(JSONB)
    kind: Mapped[str] = mapped_column(String(20))
    rule: Mapped[dict[str, Any]] = mapped_column(JSONB)
    timezone: Mapped[str] = mapped_column(String(80), default="UTC")
    status: Mapped[str] = mapped_column(String(30), default="active")
    next_fire_at: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True
    )
    next_attempt_at: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True
    )
    misfire_policy: Mapped[str] = mapped_column(String(20), default="run_once")
    max_catch_up: Mapped[int] = mapped_column(Integer, default=0)
    failure_count: Mapped[int] = mapped_column(Integer, default=0)
    last_error: Mapped[str | None] = mapped_column(Text, nullable=True)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    lease_until: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True
    )
    leased_by: Mapped[str | None] = mapped_column(String(100), nullable=True)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class ScheduleFire(Base):
    __tablename__ = "sys_schedule_fires"
    __table_args__ = (
        CheckConstraint(
            "trigger_kind IN ('scheduled', 'manual')",
            name="ck_sys_schedule_fire_trigger_kind",
        ),
        CheckConstraint(
            "status IN ('dispatched')",
            name="ck_sys_schedule_fire_status",
        ),
        ForeignKeyConstraint(
            ("schedule_id", "workspace_id"),
            ("sys_schedules.id", "sys_schedules.workspace_id"),
            name="fk_sys_schedule_fire_schedule_workspace",
        ),
        ForeignKeyConstraint(
            ("task_id", "workspace_id"),
            ("prod_tasks.id", "prod_tasks.workspace_id"),
            name="fk_sys_schedule_fire_task_workspace",
        ),
        ForeignKeyConstraint(
            ("outbox_event_id", "workspace_id"),
            ("sys_outbox_events.id", "sys_outbox_events.workspace_id"),
            name="fk_sys_schedule_fire_outbox_workspace",
        ),
        UniqueConstraint(
            "schedule_id", "fire_key", name="uq_sys_schedule_fire_key"
        ),
        Index("ix_sys_schedule_fire_schedule_created", "schedule_id", "created_at"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    schedule_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    fire_key: Mapped[str] = mapped_column(String(200))
    scheduled_for: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    trigger_kind: Mapped[str] = mapped_column(String(20))
    status: Mapped[str] = mapped_column(String(20), default="dispatched")
    task_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    outbox_event_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    trace_id: Mapped[str] = mapped_column(String(64))
    error_summary: Mapped[str | None] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
