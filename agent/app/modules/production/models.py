from datetime import UTC, datetime
from decimal import Decimal
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
    Numeric,
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


class ModelCapability(Base):
    __tablename__ = "prod_model_capabilities"
    __table_args__ = (
        CheckConstraint("kind IN ('text', 'image', 'video')", name="ck_prod_capability_kind"),
        CheckConstraint(
            "status IN ('active', 'inactive', 'unavailable')",
            name="ck_prod_capability_status",
        ),
        CheckConstraint("config_version >= 1", name="ck_prod_capability_config_version"),
        UniqueConstraint(
            "provider",
            "model",
            "kind",
            "config_version",
            name="uq_prod_capability_configuration",
        ),
        UniqueConstraint(
            "id",
            "config_version",
            name="uq_prod_capability_id_version",
        ),
        Index("ix_prod_capability_kind_status", "kind", "status"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    provider: Mapped[str] = mapped_column(String(60))
    model: Mapped[str] = mapped_column(String(160))
    kind: Mapped[str] = mapped_column(String(20))
    config_version: Mapped[int] = mapped_column(Integer)
    input_types: Mapped[list[str]] = mapped_column(JSONB)
    parameter_schema: Mapped[dict[str, Any]] = mapped_column(JSONB)
    limits: Mapped[dict[str, Any]] = mapped_column(JSONB)
    pricing: Mapped[dict[str, Any] | None] = mapped_column(JSONB, nullable=True)
    status: Mapped[str] = mapped_column(String(20))
    unavailable_reason: Mapped[str | None] = mapped_column(String(100), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class GenerationRequest(Base):
    __tablename__ = "prod_generation_requests"
    __table_args__ = (
        ForeignKeyConstraint(
            ("project_id", "workspace_id"),
            ("prj_projects.id", "prj_projects.workspace_id"),
            name="fk_prod_request_project_workspace",
        ),
        ForeignKeyConstraint(
            ("episode_id", "workspace_id"),
            ("prj_episodes.id", "prj_episodes.workspace_id"),
            name="fk_prod_request_episode_workspace",
        ),
        ForeignKeyConstraint(
            ("shot_id", "workspace_id"),
            ("sbd_shots.id", "sbd_shots.workspace_id"),
            name="fk_prod_request_shot_workspace",
        ),
        ForeignKeyConstraint(
            ("shot_spec_version_id", "workspace_id"),
            ("sbd_shot_spec_versions.id", "sbd_shot_spec_versions.workspace_id"),
            name="fk_prod_request_spec_workspace",
        ),
        CheckConstraint(
            "capability_config_version >= 1",
            name="ck_prod_request_capability_version",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_prod_request_id_workspace"),
        UniqueConstraint(
            "workspace_id",
            "idempotency_key",
            name="uq_prod_request_workspace_idempotency",
        ),
        Index("ix_prod_request_shot_created", "shot_id", "created_at"),
        Index("ix_prod_request_project_created", "project_id", "created_at"),
        Index("ix_prod_request_input_hash", "input_hash"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    shot_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    shot_spec_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    capability_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("prod_model_capabilities.id"), nullable=False
    )
    capability_config_version: Mapped[int] = mapped_column(Integer)
    parameter_snapshot: Mapped[dict[str, Any]] = mapped_column(JSONB)
    warning_acknowledgements: Mapped[list[str]] = mapped_column(JSONB)
    shot_spec_input_hash: Mapped[str] = mapped_column(String(64))
    input_hash: Mapped[str] = mapped_column(String(64))
    preflight_hash: Mapped[str] = mapped_column(String(64))
    preflight_expires_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    high_cost_confirmed: Mapped[bool] = mapped_column(Boolean, default=False)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    requested_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class GenerationRequestAsset(Base):
    __tablename__ = "prod_generation_request_assets"
    __table_args__ = (
        ForeignKeyConstraint(
            ("request_id", "workspace_id"),
            ("prod_generation_requests.id", "prod_generation_requests.workspace_id"),
            name="fk_prod_request_asset_request_workspace",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ("asset_version_id", "workspace_id"),
            ("ast_asset_versions.id", "ast_asset_versions.workspace_id"),
            name="fk_prod_request_asset_version_workspace",
        ),
        UniqueConstraint(
            "request_id",
            "slot_key",
            name="uq_prod_request_asset_slot",
        ),
        Index("ix_prod_request_asset_version", "asset_version_id"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    request_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    asset_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    slot_key: Mapped[str] = mapped_column(String(100))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class Reservation(Base):
    __tablename__ = "prod_reservations"
    __table_args__ = (
        ForeignKeyConstraint(
            ("request_id", "workspace_id"),
            ("prod_generation_requests.id", "prod_generation_requests.workspace_id"),
            name="fk_prod_reservation_request_workspace",
        ),
        CheckConstraint(
            "status IN ('active', 'settled', 'released')",
            name="ck_prod_reservation_status",
        ),
        CheckConstraint("estimated_amount >= 0", name="ck_prod_reservation_estimated"),
        CheckConstraint("reserved_amount >= 0", name="ck_prod_reservation_reserved"),
        CheckConstraint("revision >= 1", name="ck_prod_reservation_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_prod_reservation_id_workspace"),
        UniqueConstraint("request_id", name="uq_prod_reservation_request"),
        Index("ix_prod_reservation_workspace_status", "workspace_id", "status"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    request_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    currency: Mapped[str] = mapped_column(String(3))
    estimated_amount: Mapped[Decimal] = mapped_column(Numeric(20, 6))
    reserved_amount: Mapped[Decimal] = mapped_column(Numeric(20, 6))
    status: Mapped[str] = mapped_column(String(20), default="active")
    revision: Mapped[int] = mapped_column(Integer, default=1)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class CostEntry(Base):
    __tablename__ = "prod_cost_entries"
    __table_args__ = (
        ForeignKeyConstraint(
            ("reservation_id", "workspace_id"),
            ("prod_reservations.id", "prod_reservations.workspace_id"),
            name="fk_prod_cost_reservation_workspace",
        ),
        ForeignKeyConstraint(
            ("attempt_id", "workspace_id"),
            ("prod_attempts.id", "prod_attempts.workspace_id"),
            name="fk_prod_cost_attempt_workspace",
        ),
        CheckConstraint(
            "entry_type IN ('reserve', 'settle', 'release', 'adjust')",
            name="ck_prod_cost_entry_type",
        ),
        UniqueConstraint(
            "reservation_id",
            "idempotency_key",
            name="uq_prod_cost_entry_idempotency",
        ),
        Index("ix_prod_cost_reservation_created", "reservation_id", "created_at"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    reservation_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    attempt_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    entry_type: Mapped[str] = mapped_column(String(20))
    amount: Mapped[Decimal] = mapped_column(Numeric(20, 6))
    currency: Mapped[str] = mapped_column(String(3))
    provider_bill_ref: Mapped[str | None] = mapped_column(String(200), nullable=True)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


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


class GenerationAttempt(Base):
    __tablename__ = "prod_attempts"
    __table_args__ = (
        ForeignKeyConstraint(
            ("task_id", "workspace_id"),
            ("prod_tasks.id", "prod_tasks.workspace_id"),
            name="fk_prod_attempt_task_workspace",
        ),
        CheckConstraint("sequence >= 1", name="ck_prod_attempt_sequence"),
        CheckConstraint(
            "status IN ('prepared', 'submitting', 'accepted', 'polling', "
            "'succeeded', 'failed', 'cancelled', 'unknown')",
            name="ck_prod_attempt_status",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_prod_attempt_id_workspace"),
        UniqueConstraint("task_id", "sequence", name="uq_prod_attempt_task_sequence"),
        UniqueConstraint(
            "provider_request_key",
            name="uq_prod_attempt_provider_request_key",
        ),
        Index("ix_prod_attempt_workspace_status", "workspace_id", "status"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    task_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    sequence: Mapped[int] = mapped_column(Integer)
    provider_request_key: Mapped[str] = mapped_column(String(64))
    provider_task_id: Mapped[str | None] = mapped_column(String(200), nullable=True)
    status: Mapped[str] = mapped_column(String(30), default="prepared")
    request_snapshot_hash: Mapped[str] = mapped_column(String(64))
    error_code: Mapped[str | None] = mapped_column(String(80), nullable=True)
    reconcile_summary: Mapped[str | None] = mapped_column(Text, nullable=True)
    prepared_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    submitted_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    completed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )
