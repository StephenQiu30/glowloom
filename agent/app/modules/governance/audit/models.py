from datetime import UTC, datetime
from typing import Any
from uuid import UUID

from sqlalchemy import CheckConstraint, DateTime, ForeignKey, Index, String, Uuid
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class AuditEvent(Base):
    __tablename__ = "gov_audit_events"
    __table_args__ = (
        CheckConstraint(
            "result IN ('succeeded', 'denied', 'failed')",
            name="ck_gov_audit_result",
        ),
        Index(
            "ix_gov_audit_workspace_occurred",
            "workspace_id",
            "occurred_at",
        ),
        Index(
            "ix_gov_audit_workspace_actor_occurred",
            "workspace_id",
            "actor_id",
            "occurred_at",
        ),
        Index(
            "ix_gov_audit_workspace_target_occurred",
            "workspace_id",
            "target_type",
            "target_id",
            "occurred_at",
        ),
        Index(
            "ix_gov_audit_workspace_action_occurred",
            "workspace_id",
            "action",
            "occurred_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    actor_id: Mapped[UUID] = mapped_column(Uuid, ForeignKey("idn_user_accounts.id"), nullable=False)
    action: Mapped[str] = mapped_column(String(80), nullable=False)
    target_type: Mapped[str] = mapped_column(String(60), nullable=False)
    target_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    result: Mapped[str] = mapped_column(String(20), nullable=False)
    trace_id: Mapped[str] = mapped_column(String(64), nullable=False)
    event_metadata: Mapped[dict[str, Any]] = mapped_column(
        "metadata", JSONB, nullable=False, default=dict
    )
    occurred_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), nullable=False, default=_utc_now
    )
