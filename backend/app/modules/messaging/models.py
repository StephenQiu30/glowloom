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


class OutboxEvent(Base):
    __tablename__ = "sys_outbox_events"
    __table_args__ = (
        CheckConstraint(
            "status IN ('pending', 'claimed', 'published', 'manual_attention')",
            name="ck_sys_outbox_status",
        ),
        CheckConstraint("schema_version >= 1", name="ck_sys_outbox_schema_version"),
        CheckConstraint("attempt_count >= 0", name="ck_sys_outbox_attempt_count"),
        UniqueConstraint(
            "event_type",
            "aggregate_id",
            "schema_version",
            name="uq_sys_outbox_aggregate_event",
        ),
        Index("ix_sys_outbox_publishable", "status", "available_at"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    event_type: Mapped[str] = mapped_column(String(100))
    schema_version: Mapped[int] = mapped_column(Integer)
    aggregate_type: Mapped[str] = mapped_column(String(50))
    aggregate_id: Mapped[UUID] = mapped_column(Uuid)
    routing_key: Mapped[str] = mapped_column(String(100))
    payload: Mapped[dict[str, Any]] = mapped_column(JSONB)
    trace_id: Mapped[str] = mapped_column(String(64))
    causation_event_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    status: Mapped[str] = mapped_column(String(30), default="pending")
    attempt_count: Mapped[int] = mapped_column(Integer, default=0)
    available_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    claimed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    claimed_by: Mapped[str | None] = mapped_column(String(100), nullable=True)
    published_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    last_error: Mapped[str | None] = mapped_column(Text, nullable=True)
    occurred_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class InboxDelivery(Base):
    __tablename__ = "sys_inbox_deliveries"
    __table_args__ = (
        CheckConstraint(
            "status IN ('processing', 'completed', 'rejected', "
            "'retry_scheduled', 'manual_attention')",
            name="ck_sys_inbox_status",
        ),
        CheckConstraint("attempt_count >= 1", name="ck_sys_inbox_attempt_count"),
        ForeignKeyConstraint(
            ["task_id", "workspace_id"],
            ["prod_tasks.id", "prod_tasks.workspace_id"],
            name="fk_sys_inbox_task_workspace",
        ),
        UniqueConstraint(
            "event_id",
            "consumer_name",
            name="uq_sys_inbox_event_consumer",
        ),
        Index("ix_sys_inbox_status_received", "status", "received_at"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    event_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    event_type: Mapped[str] = mapped_column(String(100), nullable=False)
    consumer_name: Mapped[str] = mapped_column(String(100), nullable=False)
    task_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    trace_id: Mapped[str] = mapped_column(String(64), nullable=False)
    status: Mapped[str] = mapped_column(String(30), default="processing")
    attempt_count: Mapped[int] = mapped_column(Integer, default=1)
    received_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    processed_at: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True
    )
    last_error: Mapped[str | None] = mapped_column(String(80), nullable=True)
