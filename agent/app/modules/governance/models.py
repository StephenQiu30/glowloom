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


class Consent(Base):
    __tablename__ = "gov_consents"
    __table_args__ = (
        CheckConstraint(
            "status IN ('active', 'expired', 'revoked')",
            name="ck_gov_consent_status",
        ),
        CheckConstraint(
            "subject_type IN ('SCRIPT_VERSION', 'ASSET_VERSION', "
            "'SHOT_SPEC_VERSION', 'CANDIDATE', 'MEDIA_VERSION', "
            "'TIMELINE_VERSION', 'DELIVERY')",
            name="ck_gov_consent_subject_type",
        ),
        CheckConstraint("revision >= 1", name="ck_gov_consent_revision"),
        ForeignKeyConstraint(
            ("current_revision_id", "workspace_id"),
            ("gov_consent_revisions.id", "gov_consent_revisions.workspace_id"),
            name="fk_gov_consent_current_revision_workspace",
            deferrable=True,
            initially="DEFERRED",
            use_alter=True,
        ),
        UniqueConstraint("id", "workspace_id", name="uq_gov_consent_id_workspace"),
        UniqueConstraint(
            "workspace_id",
            "idempotency_key",
            name="uq_gov_consent_workspace_idempotency",
        ),
        Index(
            "ix_gov_consent_workspace_status_updated",
            "workspace_id",
            "status",
            "updated_at",
        ),
        Index(
            "ix_gov_consent_workspace_subject",
            "workspace_id",
            "subject_type",
            "subject_id",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    subject_identity: Mapped[dict[str, Any]] = mapped_column(JSONB)
    subject_type: Mapped[str] = mapped_column(String(40))
    subject_id: Mapped[UUID] = mapped_column(Uuid)
    status: Mapped[str] = mapped_column(String(20), default="active")
    current_revision_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class ConsentRevision(Base):
    __tablename__ = "gov_consent_revisions"
    __table_args__ = (
        ForeignKeyConstraint(
            ("consent_id", "workspace_id"),
            ("gov_consents.id", "gov_consents.workspace_id"),
            name="fk_gov_revision_consent_workspace",
            deferrable=True,
            initially="DEFERRED",
        ),
        CheckConstraint("revision_no >= 1", name="ck_gov_revision_number"),
        CheckConstraint(
            "action IN ('register', 'update', 'revoke')",
            name="ck_gov_revision_action",
        ),
        CheckConstraint("valid_to > valid_from", name="ck_gov_revision_validity"),
        UniqueConstraint("id", "workspace_id", name="uq_gov_revision_id_workspace"),
        UniqueConstraint("consent_id", "revision_no", name="uq_gov_revision_consent_number"),
        Index(
            "ix_gov_revision_consent_created",
            "consent_id",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    consent_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    revision_no: Mapped[int] = mapped_column(Integer)
    action: Mapped[str] = mapped_column(String(20))
    scope: Mapped[dict[str, Any]] = mapped_column(JSONB)
    valid_from: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    valid_to: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    reason: Mapped[str] = mapped_column(Text)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class ConsentProof(Base):
    __tablename__ = "gov_consent_proofs"
    __table_args__ = (
        ForeignKeyConstraint(
            ("consent_revision_id", "workspace_id"),
            ("gov_consent_revisions.id", "gov_consent_revisions.workspace_id"),
            name="fk_gov_proof_revision_workspace",
        ),
        ForeignKeyConstraint(
            ("media_version_id", "workspace_id"),
            ("med_media_versions.id", "med_media_versions.workspace_id"),
            name="fk_gov_proof_media_workspace",
        ),
        CheckConstraint("position >= 1", name="ck_gov_proof_position"),
        UniqueConstraint(
            "consent_revision_id",
            "media_version_id",
            name="uq_gov_proof_revision_media",
        ),
        UniqueConstraint("consent_revision_id", "position", name="uq_gov_proof_revision_position"),
        Index("ix_gov_proof_media", "media_version_id"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    consent_revision_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    media_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    purpose: Mapped[str] = mapped_column(String(40), default="authorization_evidence")
    position: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
