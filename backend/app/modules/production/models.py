from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy import (
    Boolean,
    CheckConstraint,
    DateTime,
    ForeignKey,
    Index,
    Integer,
    String,
    Text,
    UniqueConstraint,
    Uuid,
)
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class Task(Base):
    __tablename__ = "prod_tasks"
    __table_args__ = (
        CheckConstraint(
            "status IN ('queued', 'running', 'waiting_provider', 'succeeded', "
            "'failed', 'cancelled', 'unknown')",
            name="ck_prod_task_status",
        ),
        CheckConstraint(
            "cancel_status IN ('none', 'requested', 'accepted', 'rejected')",
            name="ck_prod_task_cancel_status",
        ),
        CheckConstraint("revision >= 1", name="ck_prod_task_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_prod_task_id_workspace"),
        UniqueConstraint(
            "workspace_id",
            "task_type",
            "idempotency_key",
            name="uq_prod_task_idempotency",
        ),
        Index(
            "ix_prod_task_workspace_status_created",
            "workspace_id",
            "status",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    task_type: Mapped[str] = mapped_column(String(50))
    request_type: Mapped[str] = mapped_column(String(50))
    request_id: Mapped[UUID] = mapped_column(Uuid)
    episode_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    render_snapshot_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    usage_type: Mapped[str | None] = mapped_column(String(50), nullable=True)
    usage_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    input_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    input_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    status: Mapped[str] = mapped_column(String(30), default="queued")
    progress_stage: Mapped[str] = mapped_column(String(50), default="queued")
    error_code: Mapped[str | None] = mapped_column(String(80), nullable=True)
    error_retryable: Mapped[bool | None] = mapped_column(Boolean, nullable=True)
    error_summary: Mapped[str | None] = mapped_column(Text, nullable=True)
    next_action: Mapped[str | None] = mapped_column(String(80), nullable=True)
    cancel_status: Mapped[str] = mapped_column(String(20), default="none")
    idempotency_key: Mapped[str] = mapped_column(String(200))
    requested_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    revision: Mapped[int] = mapped_column(Integer, default=1)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )
