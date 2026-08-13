from datetime import UTC, datetime
from typing import Any
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
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class StoryboardExportJob(Base):
    __tablename__ = "sbd_export_jobs"
    __table_args__ = (
        ForeignKeyConstraint(
            ("project_id", "workspace_id"),
            ("prj_projects.id", "prj_projects.workspace_id"),
            name="fk_sbd_export_job_project",
        ),
        ForeignKeyConstraint(
            ("episode_id", "workspace_id"),
            ("prj_episodes.id", "prj_episodes.workspace_id"),
            name="fk_sbd_export_job_episode",
        ),
        ForeignKeyConstraint(
            ("task_id", "workspace_id"),
            ("prod_tasks.id", "prod_tasks.workspace_id"),
            name="fk_sbd_export_job_task",
            deferrable=True,
            initially="DEFERRED",
        ),
        CheckConstraint(
            "status IN ('queued', 'running', 'succeeded', 'failed')",
            name="ck_sbd_export_job_status",
        ),
        CheckConstraint("schema_version = 1", name="ck_sbd_export_job_schema"),
        UniqueConstraint("id", "workspace_id", name="uq_sbd_export_job_workspace"),
        UniqueConstraint(
            "id",
            "episode_id",
            "workspace_id",
            name="uq_sbd_export_job_scope",
        ),
        UniqueConstraint("task_id", name="uq_sbd_export_job_task"),
        UniqueConstraint(
            "episode_id",
            "idempotency_key",
            name="uq_sbd_export_job_idempotency",
        ),
        Index("ix_sbd_export_job_episode_created", "episode_id", "created_at"),
        Index("ix_sbd_export_job_workspace_status", "workspace_id", "status"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    schema_version: Mapped[int] = mapped_column(Integer, default=1)
    input_hash: Mapped[str] = mapped_column(String(64))
    input_snapshot: Mapped[dict[str, Any]] = mapped_column(JSONB)
    command_hash: Mapped[str] = mapped_column(String(64))
    idempotency_key: Mapped[str] = mapped_column(String(200))
    task_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    status: Mapped[str] = mapped_column(String(20), default="queued")
    error_code: Mapped[str | None] = mapped_column(String(80), nullable=True)
    error_summary: Mapped[str | None] = mapped_column(Text, nullable=True)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class StoryboardExportManifest(Base):
    __tablename__ = "sbd_export_manifests"
    __table_args__ = (
        ForeignKeyConstraint(
            ("job_id", "episode_id", "workspace_id"),
            (
                "sbd_export_jobs.id",
                "sbd_export_jobs.episode_id",
                "sbd_export_jobs.workspace_id",
            ),
            name="fk_sbd_export_manifest_job",
        ),
        ForeignKeyConstraint(
            ("media_version_id", "workspace_id"),
            ("med_media_versions.id", "med_media_versions.workspace_id"),
            name="fk_sbd_export_manifest_media",
        ),
        CheckConstraint("schema_version = 1", name="ck_sbd_export_manifest_schema"),
        CheckConstraint("package_size_bytes >= 1", name="ck_sbd_export_manifest_size"),
        UniqueConstraint(
            "id", "workspace_id", name="uq_sbd_export_manifest_workspace"
        ),
        UniqueConstraint("job_id", name="uq_sbd_export_manifest_job"),
        UniqueConstraint(
            "media_version_id", name="uq_sbd_export_manifest_media"
        ),
        Index(
            "ix_sbd_export_manifest_episode_created", "episode_id", "created_at"
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    job_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    schema_version: Mapped[int] = mapped_column(Integer, default=1)
    input_hash: Mapped[str] = mapped_column(String(64))
    input_snapshot: Mapped[dict[str, Any]] = mapped_column(JSONB)
    file_manifest: Mapped[dict[str, Any]] = mapped_column(JSONB)
    media_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    package_sha256: Mapped[str] = mapped_column(String(64))
    package_size_bytes: Mapped[int] = mapped_column(BigInteger)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
