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
        ForeignKeyConstraint(
            ("current_version_id", "workspace_id"),
            ("ast_asset_versions.id", "ast_asset_versions.workspace_id"),
            name="fk_ast_asset_current_version_workspace",
            deferrable=True,
            initially="DEFERRED",
            use_alter=True,
        ),
        CheckConstraint(
            "kind IN ('character', 'location', 'prop', 'costume', "
            "'visual_style', 'voice')",
            name="ck_ast_asset_kind",
        ),
        CheckConstraint(
            "status IN ('active', 'archived')", name="ck_ast_asset_status"
        ),
        CheckConstraint("revision >= 1", name="ck_ast_asset_revision"),
        UniqueConstraint("id", "workspace_id", name="uq_ast_asset_id_workspace"),
        Index(
            "ix_ast_asset_project_kind_status",
            "project_id",
            "kind",
            "status",
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
    current_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    archived_at: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True
    )
    archived_by: Mapped[UUID | None] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=True
    )
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class AssetVersion(Base):
    __tablename__ = "ast_asset_versions"
    __table_args__ = (
        ForeignKeyConstraint(
            ("asset_id", "workspace_id"),
            ("ast_assets.id", "ast_assets.workspace_id"),
            name="fk_ast_version_asset_workspace",
            deferrable=True,
            initially="DEFERRED",
        ),
        CheckConstraint("version_no >= 1", name="ck_ast_version_number"),
        CheckConstraint("schema_version >= 1", name="ck_ast_schema_version"),
        CheckConstraint(
            "source_type IN ('manual', 'candidate')",
            name="ck_ast_version_source_type",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_ast_version_id_workspace"),
        UniqueConstraint("asset_id", "version_no", name="uq_ast_version_number"),
        UniqueConstraint(
            "source_type", "source_id", name="uq_ast_version_source"
        ),
        Index("ix_ast_version_content_hash", "content_hash"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    asset_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
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
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now
    )


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
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now
    )
