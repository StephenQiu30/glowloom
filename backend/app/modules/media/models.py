from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy import (
    BigInteger,
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
    text,
)
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class MediaObject(Base):
    __tablename__ = "med_media_objects"
    __table_args__ = (
        CheckConstraint(
            "kind IN ('image', 'video', 'audio', 'subtitle', 'delivery', 'document')",
            name="ck_med_media_object_kind",
        ),
        CheckConstraint(
            "source_type IN ('upload', 'generated', 'rendered')",
            name="ck_med_media_object_source",
        ),
        CheckConstraint(
            "status IN ('active', 'archived')",
            name="ck_med_media_object_status",
        ),
        CheckConstraint("revision >= 1", name="ck_med_media_object_revision"),
        ForeignKeyConstraint(
            ("current_version_id", "workspace_id"),
            ("med_media_versions.id", "med_media_versions.workspace_id"),
            name="fk_med_object_current_version_workspace",
            deferrable=True,
            initially="DEFERRED",
            use_alter=True,
        ),
        UniqueConstraint("id", "workspace_id", name="uq_med_object_id_workspace"),
        Index(
            "ix_med_object_workspace_kind_status_created",
            "workspace_id",
            "kind",
            "status",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    kind: Mapped[str] = mapped_column(String(30))
    source_type: Mapped[str] = mapped_column(String(30))
    status: Mapped[str] = mapped_column(String(20), default="active")
    current_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    archived_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    archived_by: Mapped[UUID | None] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=True
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class MediaVersion(Base):
    __tablename__ = "med_media_versions"
    __table_args__ = (
        ForeignKeyConstraint(
            ("media_object_id", "workspace_id"),
            ("med_media_objects.id", "med_media_objects.workspace_id"),
            name="fk_med_version_object_workspace",
            deferrable=True,
            initially="DEFERRED",
        ),
        CheckConstraint("version_no >= 1", name="ck_med_version_number"),
        CheckConstraint("size_bytes >= 1", name="ck_med_version_size"),
        CheckConstraint("probe_attempt >= 1", name="ck_med_version_probe_attempt"),
        CheckConstraint(
            "probe_status IN ('pending', 'ready', 'failed', 'quarantined')",
            name="ck_med_version_probe_status",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_med_version_id_workspace"),
        UniqueConstraint("media_object_id", "version_no", name="uq_med_version_object_number"),
        Index("ix_med_version_workspace_created", "workspace_id", "created_at"),
        Index("ix_med_version_sha256", "sha256"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    media_object_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    version_no: Mapped[int] = mapped_column(Integer)
    filename: Mapped[str] = mapped_column(String(255))
    sha256: Mapped[str] = mapped_column(String(64))
    size_bytes: Mapped[int] = mapped_column(BigInteger)
    mime_type: Mapped[str] = mapped_column(String(120))
    probe_status: Mapped[str] = mapped_column(String(20), default="pending")
    probe_attempt: Mapped[int] = mapped_column(Integer, default=1)
    probe_task_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    probe_idempotency_key: Mapped[str | None] = mapped_column(String(200), nullable=True)
    probe_error_code: Mapped[str | None] = mapped_column(String(80), nullable=True)
    probe_error_summary: Mapped[str | None] = mapped_column(Text, nullable=True)
    probe_next_action: Mapped[str | None] = mapped_column(String(80), nullable=True)
    width: Mapped[int | None] = mapped_column(Integer, nullable=True)
    height: Mapped[int | None] = mapped_column(Integer, nullable=True)
    duration_ms: Mapped[int | None] = mapped_column(BigInteger, nullable=True)
    codec: Mapped[str | None] = mapped_column(String(120), nullable=True)
    container: Mapped[str | None] = mapped_column(String(120), nullable=True)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class MediaLocation(Base):
    __tablename__ = "med_media_locations"
    __table_args__ = (
        ForeignKeyConstraint(
            ("media_version_id", "workspace_id"),
            ("med_media_versions.id", "med_media_versions.workspace_id"),
            name="fk_med_location_version_workspace",
            deferrable=True,
            initially="DEFERRED",
        ),
        CheckConstraint(
            "status IN ('verified', 'active', 'retiring', 'retired', 'quarantined')",
            name="ck_med_location_status",
        ),
        UniqueConstraint(
            "storage_profile",
            "bucket",
            "object_key",
            name="uq_med_location_physical_object",
        ),
        Index(
            "uq_med_location_active_version",
            "media_version_id",
            unique=True,
            postgresql_where=text("status = 'active'"),
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    media_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    storage_profile: Mapped[str] = mapped_column(String(80))
    bucket: Mapped[str] = mapped_column(String(255))
    object_key: Mapped[str] = mapped_column(String(1024))
    status: Mapped[str] = mapped_column(String(20), default="active")
    verified_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    migration_task_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    retire_after: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    retired_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class MediaLineage(Base):
    __tablename__ = "med_media_lineages"
    __table_args__ = (
        ForeignKeyConstraint(
            ("media_version_id", "workspace_id"),
            ("med_media_versions.id", "med_media_versions.workspace_id"),
            name="fk_med_lineage_version_workspace",
        ),
        CheckConstraint("position >= 1", name="ck_med_lineage_position"),
        CheckConstraint(
            "source_type IN ('asset_version', 'narrative_unit_version', "
            "'script_version', 'shot_spec_version', 'storyboard_coverage', "
            "'storyboard_export_snapshot', 'storyboard_readiness')",
            name="ck_med_lineage_source_type",
        ),
        UniqueConstraint(
            "media_version_id",
            "position",
            name="uq_med_lineage_version_position",
        ),
        UniqueConstraint(
            "media_version_id",
            "source_type",
            "source_id",
            name="uq_med_lineage_source",
        ),
        Index("ix_med_lineage_source", "source_type", "source_id"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    media_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    source_type: Mapped[str] = mapped_column(String(40))
    source_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    source_hash: Mapped[str] = mapped_column(String(64))
    position: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class UploadSession(Base):
    __tablename__ = "med_upload_sessions"
    __table_args__ = (
        CheckConstraint(
            "declared_kind IN ('image', 'video', 'audio', 'subtitle', 'delivery', 'document')",
            name="ck_med_upload_kind",
        ),
        CheckConstraint(
            "status IN ('pending', 'completed', 'expired', 'failed')",
            name="ck_med_upload_status",
        ),
        CheckConstraint("declared_size_bytes >= 1", name="ck_med_upload_size"),
        UniqueConstraint("workspace_id", "idempotency_key", name="uq_med_upload_idempotency"),
        UniqueConstraint(
            "storage_profile",
            "bucket",
            "object_key",
            name="uq_med_upload_physical_object",
        ),
        Index("ix_med_upload_workspace_status_expiry", "workspace_id", "status", "expires_at"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    media_object_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    expected_current_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    storage_profile: Mapped[str] = mapped_column(String(80))
    bucket: Mapped[str] = mapped_column(String(255))
    object_key: Mapped[str] = mapped_column(String(1024))
    filename: Mapped[str] = mapped_column(String(255))
    declared_kind: Mapped[str] = mapped_column(String(30))
    declared_size_bytes: Mapped[int] = mapped_column(BigInteger)
    declared_mime_type: Mapped[str] = mapped_column(String(120))
    declared_sha256: Mapped[str] = mapped_column(String(64))
    status: Mapped[str] = mapped_column(String(20), default="pending")
    expires_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    idempotency_key: Mapped[str] = mapped_column(String(200))
    completed_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    completed_probe_task_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    error_code: Mapped[str | None] = mapped_column(String(80), nullable=True)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )
