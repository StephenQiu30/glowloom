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
from sqlalchemy.dialects.postgresql import ARRAY, JSONB
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class Asset(Base):
    __tablename__ = "ast_assets"
    __table_args__ = (
        ForeignKeyConstraint(
            ("project_id", "workspace_id"),
            ("prj_projects.id", "prj_projects.workspace_id"),
            name="fk_ast_asset_project_workspace",
        ),
        CheckConstraint(
            "kind IN ('character', 'location', 'prop', 'costume', 'visual_style', 'voice')",
            name="ck_ast_asset_kind",
        ),
        CheckConstraint("status IN ('active', 'archived')", name="ck_ast_asset_status"),
        CheckConstraint(
            "availability IN ('enabled', 'disabled')",
            name="ck_ast_asset_availability",
        ),
        CheckConstraint("revision >= 1", name="ck_ast_asset_revision"),
        CheckConstraint("name_revision >= 1", name="ck_ast_asset_name_revision"),
        ForeignKeyConstraint(
            ("id", "workspace_id", "name_revision"),
            (
                "ast_asset_name_revisions.asset_id",
                "ast_asset_name_revisions.workspace_id",
                "ast_asset_name_revisions.revision_no",
            ),
            name="fk_ast_asset_current_name",
            deferrable=True,
            initially="DEFERRED",
            use_alter=True,
        ),
        UniqueConstraint("id", "workspace_id", name="uq_ast_asset_id_workspace"),
        Index(
            "ix_ast_asset_project_kind_status",
            "project_id",
            "kind",
            "status",
        ),
        Index(
            "ix_ast_asset_project_availability",
            "project_id",
            "availability",
        ),
        Index(
            "ix_ast_asset_project_kind_normalized_name",
            "project_id",
            "kind",
            "normalized_name",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    kind: Mapped[str] = mapped_column(String(30))
    name: Mapped[str] = mapped_column(String(200))
    normalized_name: Mapped[str] = mapped_column(String(200))
    aliases: Mapped[list[str]] = mapped_column(ARRAY(Text), default=list)
    tags: Mapped[list[str]] = mapped_column(ARRAY(Text), default=list)
    status: Mapped[str] = mapped_column(String(20), default="active")
    availability: Mapped[str] = mapped_column(String(20), default="enabled")
    name_revision: Mapped[int] = mapped_column(Integer, default=1)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    command_receipts: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    archived_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    archived_by: Mapped[UUID | None] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=True
    )
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class AssetNameRevision(Base):
    __tablename__ = "ast_asset_name_revisions"
    __table_args__ = (
        ForeignKeyConstraint(
            ("asset_id", "workspace_id"),
            ("ast_assets.id", "ast_assets.workspace_id"),
            name="fk_ast_name_revision_asset_workspace",
            ondelete="CASCADE",
        ),
        CheckConstraint("revision_no >= 1", name="ck_ast_name_revision_number"),
        UniqueConstraint(
            "asset_id",
            "workspace_id",
            "revision_no",
            name="uq_ast_name_revision_scope",
        ),
        Index("ix_ast_name_revision_asset_created", "asset_id", "created_at"),
    )

    asset_id: Mapped[UUID] = mapped_column(Uuid, primary_key=True)
    revision_no: Mapped[int] = mapped_column(Integer, primary_key=True)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    name_snapshot: Mapped[str] = mapped_column(String(200))
    normalized_name: Mapped[str] = mapped_column(String(200))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class AssetState(Base):
    __tablename__ = "ast_asset_states"
    __table_args__ = (
        ForeignKeyConstraint(
            ("asset_id", "workspace_id"),
            ("ast_assets.id", "ast_assets.workspace_id"),
            name="fk_ast_state_asset_workspace",
        ),
        ForeignKeyConstraint(
            ("current_version_id", "id", "asset_id", "workspace_id"),
            (
                "ast_asset_versions.id",
                "ast_asset_versions.asset_state_id",
                "ast_asset_versions.asset_id",
                "ast_asset_versions.workspace_id",
            ),
            name="fk_ast_state_current_version_scope",
            deferrable=True,
            initially="DEFERRED",
            use_alter=True,
        ),
        CheckConstraint(
            "status IN ('active', 'disabled')",
            name="ck_ast_state_status",
        ),
        CheckConstraint("revision >= 1", name="ck_ast_state_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_ast_state_id_workspace"),
        UniqueConstraint(
            "id",
            "asset_id",
            "workspace_id",
            name="uq_ast_state_scope",
        ),
        UniqueConstraint(
            "asset_id",
            "state_key",
            name="uq_ast_state_asset_key",
        ),
        UniqueConstraint(
            "asset_id",
            "creation_key",
            name="uq_ast_state_asset_creation",
        ),
        Index("ix_ast_state_asset_status", "asset_id", "status"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    asset_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    state_key: Mapped[str] = mapped_column(String(80))
    label: Mapped[str] = mapped_column(String(120))
    description: Mapped[str] = mapped_column(Text, default="")
    status: Mapped[str] = mapped_column(String(20), default="active")
    current_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    creation_key: Mapped[str] = mapped_column(String(200))
    command_receipts: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class AssetVersion(Base):
    __tablename__ = "ast_asset_versions"
    __table_args__ = (
        ForeignKeyConstraint(
            ("asset_state_id", "asset_id", "workspace_id"),
            (
                "ast_asset_states.id",
                "ast_asset_states.asset_id",
                "ast_asset_states.workspace_id",
            ),
            name="fk_ast_version_state_scope",
            deferrable=True,
            initially="DEFERRED",
        ),
        CheckConstraint("version_no >= 1", name="ck_ast_version_number"),
        CheckConstraint("schema_version >= 1", name="ck_ast_schema_version"),
        CheckConstraint(
            "source_type IN ('manual', 'script_extraction_candidate', "
            "'production_bible_state')",
            name="ck_ast_version_source_type",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_ast_version_id_workspace"),
        UniqueConstraint(
            "id",
            "asset_state_id",
            "asset_id",
            "workspace_id",
            name="uq_ast_version_scope",
        ),
        UniqueConstraint("asset_id", "version_no", name="uq_ast_version_number"),
        UniqueConstraint("source_type", "source_id", name="uq_ast_version_source"),
        Index("ix_ast_version_content_hash", "content_hash"),
        Index("ix_ast_version_state_number", "asset_state_id", "version_no"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    asset_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    asset_state_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    version_no: Mapped[int] = mapped_column(Integer)
    schema_version: Mapped[int] = mapped_column(Integer, default=1)
    spec: Mapped[dict[str, Any]] = mapped_column(JSONB)
    prompt_description: Mapped[str] = mapped_column(Text, default="")
    source_type: Mapped[str] = mapped_column(String(30), default="manual")
    source_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    content_hash: Mapped[str] = mapped_column(String(64))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class AssetOccurrenceDecision(Base):
    __tablename__ = "ast_asset_occurrences"
    __table_args__ = (
        ForeignKeyConstraint(
            ("asset_state_id", "workspace_id"),
            ("ast_asset_states.id", "ast_asset_states.workspace_id"),
            name="fk_ast_occurrence_state_workspace",
        ),
        ForeignKeyConstraint(
            ("narrative_unit_id", "episode_id", "workspace_id"),
            (
                "scr_narrative_units.id",
                "scr_narrative_units.episode_id",
                "scr_narrative_units.workspace_id",
            ),
            name="fk_ast_occurrence_unit_scope",
        ),
        ForeignKeyConstraint(
            (
                "narrative_unit_version_id",
                "narrative_unit_id",
                "episode_id",
                "workspace_id",
            ),
            (
                "scr_narrative_unit_versions.id",
                "scr_narrative_unit_versions.unit_id",
                "scr_narrative_unit_versions.episode_id",
                "scr_narrative_unit_versions.workspace_id",
            ),
            name="fk_ast_occurrence_unit_version_scope",
        ),
        CheckConstraint("sequence >= 1", name="ck_ast_occurrence_sequence"),
        CheckConstraint(
            "decision IN ('link', 'unlink')",
            name="ck_ast_occurrence_decision",
        ),
        CheckConstraint(
            "origin IN ('manual', 'script_candidate')",
            name="ck_ast_occurrence_origin",
        ),
        UniqueConstraint(
            "asset_state_id",
            "sequence",
            name="uq_ast_occurrence_state_sequence",
        ),
        UniqueConstraint(
            "asset_state_id",
            "idempotency_key",
            name="uq_ast_occurrence_state_idempotency",
        ),
        Index(
            "ix_ast_occurrence_episode_state",
            "episode_id",
            "asset_state_id",
        ),
        Index(
            "ix_ast_occurrence_unit_created",
            "narrative_unit_id",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    asset_state_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    narrative_unit_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    narrative_unit_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    sequence: Mapped[int] = mapped_column(Integer)
    decision: Mapped[str] = mapped_column(String(20))
    origin: Mapped[str] = mapped_column(String(30))
    evidence_hash: Mapped[str] = mapped_column(String(64))
    idempotency_key: Mapped[str] = mapped_column(String(200))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class AssetMediaReference(Base):
    __tablename__ = "ast_media_references"
    __table_args__ = (
        ForeignKeyConstraint(
            ("asset_version_id", "workspace_id"),
            ("ast_asset_versions.id", "ast_asset_versions.workspace_id"),
            name="fk_ast_media_ref_version_workspace",
        ),
        ForeignKeyConstraint(
            ("media_version_id", "workspace_id"),
            ("med_media_versions.id", "med_media_versions.workspace_id"),
            name="fk_ast_media_ref_media_workspace",
        ),
        CheckConstraint("position >= 1", name="ck_ast_media_ref_position"),
        UniqueConstraint(
            "asset_version_id",
            "purpose",
            "position",
            name="uq_ast_media_ref_purpose_position",
        ),
        Index("ix_ast_media_ref_media", "media_version_id"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    asset_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    media_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    purpose: Mapped[str] = mapped_column(String(40))
    position: Mapped[int] = mapped_column(Integer)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
