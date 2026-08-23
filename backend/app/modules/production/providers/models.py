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
    LargeBinary,
    String,
    UniqueConstraint,
    Uuid,
    text,
)
from sqlalchemy.dialects.postgresql import JSONB
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class ProviderConnection(Base):
    __tablename__ = "prod_provider_connections"
    __table_args__ = (
        CheckConstraint("catalog_version >= 1", name="ck_prod_provider_connection_catalog"),
        CheckConstraint("revision >= 1", name="ck_prod_provider_connection_revision"),
        CheckConstraint(
            "protocol IN ('openai_compatible', 'anthropic_native', 'gemini_native', 'ark_native')",
            name="ck_prod_provider_connection_protocol",
        ),
        CheckConstraint(
            "configuration_status IN ('incomplete', 'valid', 'invalid')",
            name="ck_prod_provider_connection_configuration",
        ),
        UniqueConstraint(
            "id",
            "workspace_id",
            name="uq_prod_provider_connection_id_workspace",
        ),
        Index(
            "ix_prod_provider_connection_workspace_archived",
            "workspace_id",
            "archived_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    preset_id: Mapped[str] = mapped_column(String(100))
    catalog_version: Mapped[int] = mapped_column(Integer)
    display_name: Mapped[str] = mapped_column(String(200))
    protocol: Mapped[str] = mapped_column(String(40))
    region: Mapped[str | None] = mapped_column(String(100), nullable=True)
    base_url: Mapped[str] = mapped_column(String(2048))
    non_secret_config: Mapped[dict[str, Any]] = mapped_column(JSONB)
    configuration_status: Mapped[str] = mapped_column(String(20), default="incomplete")
    revision: Mapped[int] = mapped_column(Integer, default=1)
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    updated_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    archived_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class ProviderCredentialVersion(Base):
    __tablename__ = "prod_provider_credential_versions"
    __table_args__ = (
        ForeignKeyConstraint(
            ("connection_id", "workspace_id"),
            ("prod_provider_connections.id", "prod_provider_connections.workspace_id"),
            name="fk_prod_provider_credential_connection_workspace",
        ),
        CheckConstraint("version >= 1", name="ck_prod_provider_credential_version"),
        CheckConstraint(
            "status IN ('current', 'retiring', 'revoked')",
            name="ck_prod_provider_credential_status",
        ),
        CheckConstraint(
            "octet_length(nonce) = 12",
            name="ck_prod_provider_credential_nonce",
        ),
        CheckConstraint(
            "octet_length(ciphertext) > 0",
            name="ck_prod_provider_credential_ciphertext",
        ),
        CheckConstraint(
            "octet_length(auth_tag) = 16",
            name="ck_prod_provider_credential_auth_tag",
        ),
        CheckConstraint(
            "char_length(fingerprint_hmac) = 64",
            name="ck_prod_provider_credential_fingerprint",
        ),
        UniqueConstraint(
            "id",
            "workspace_id",
            "connection_id",
            name="uq_prod_provider_credential_identity",
        ),
        UniqueConstraint(
            "connection_id",
            "version",
            name="uq_prod_provider_credential_connection_version",
        ),
        UniqueConstraint(
            "connection_id",
            "fingerprint_hmac",
            name="uq_prod_provider_credential_connection_fingerprint",
        ),
        Index(
            "uq_prod_provider_credential_current",
            "connection_id",
            unique=True,
            postgresql_where=text("status = 'current'"),
        ),
        Index(
            "ix_prod_provider_credential_workspace_created",
            "workspace_id",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    connection_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    version: Mapped[int] = mapped_column(Integer)
    key_id: Mapped[str] = mapped_column(String(100))
    nonce: Mapped[bytes] = mapped_column(LargeBinary)
    ciphertext: Mapped[bytes] = mapped_column(LargeBinary)
    auth_tag: Mapped[bytes] = mapped_column(LargeBinary)
    fingerprint_hmac: Mapped[str] = mapped_column(String(64))
    status: Mapped[str] = mapped_column(String(20), default="current")
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    retired_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)


class ProviderBinding(Base):
    __tablename__ = "prod_provider_bindings"
    __table_args__ = (
        ForeignKeyConstraint(
            ("credential_version_id", "workspace_id", "connection_id"),
            (
                "prod_provider_credential_versions.id",
                "prod_provider_credential_versions.workspace_id",
                "prod_provider_credential_versions.connection_id",
            ),
            name="fk_prod_provider_binding_credential_identity",
        ),
        ForeignKeyConstraint(
            ("capability_id", "capability_config_version"),
            ("prod_model_capabilities.id", "prod_model_capabilities.config_version"),
            name="fk_prod_provider_binding_capability_version",
        ),
        CheckConstraint(
            "usage_type IN ('script_structure', 'image_generation', 'video_generation')",
            name="ck_prod_provider_binding_usage",
        ),
        CheckConstraint(
            "status IN ('active', 'inactive')",
            name="ck_prod_provider_binding_status",
        ),
        CheckConstraint(
            "binding_revision >= 1",
            name="ck_prod_provider_binding_revision",
        ),
        CheckConstraint(
            "capability_config_version >= 1",
            name="ck_prod_provider_binding_capability_version",
        ),
        CheckConstraint(
            "(status = 'active' AND deactivated_by IS NULL AND deactivated_at IS NULL) OR "
            "(status = 'inactive' AND deactivated_by IS NOT NULL AND deactivated_at IS NOT NULL)",
            name="ck_prod_provider_binding_lifecycle",
        ),
        UniqueConstraint(
            "id",
            "workspace_id",
            name="uq_prod_provider_binding_id_workspace",
        ),
        Index(
            "uq_prod_provider_binding_active_usage",
            "workspace_id",
            "usage_type",
            unique=True,
            postgresql_where=text("status = 'active'"),
        ),
        Index(
            "ix_prod_provider_binding_connection_status",
            "connection_id",
            "status",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    usage_type: Mapped[str] = mapped_column(String(40))
    connection_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    credential_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    capability_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    capability_config_version: Mapped[int] = mapped_column(Integer)
    binding_revision: Mapped[int] = mapped_column(Integer, default=1)
    status: Mapped[str] = mapped_column(String(20), default="active")
    activated_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    activated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    deactivated_by: Mapped[UUID | None] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=True
    )
    deactivated_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)


class ProviderHealthCheck(Base):
    __tablename__ = "prod_provider_health_checks"
    __table_args__ = (
        ForeignKeyConstraint(
            ("credential_version_id", "workspace_id", "connection_id"),
            (
                "prod_provider_credential_versions.id",
                "prod_provider_credential_versions.workspace_id",
                "prod_provider_credential_versions.connection_id",
            ),
            name="fk_prod_provider_health_credential_identity",
        ),
        CheckConstraint(
            "connection_revision >= 1",
            name="ck_prod_provider_health_connection_revision",
        ),
        CheckConstraint(
            "probe_type IN ('model_discovery', 'metadata')",
            name="ck_prod_provider_health_probe_type",
        ),
        CheckConstraint(
            "status IN ('healthy', 'degraded', 'unreachable')",
            name="ck_prod_provider_health_status",
        ),
        CheckConstraint(
            "latency_ms IS NULL OR latency_ms >= 0",
            name="ck_prod_provider_health_latency",
        ),
        CheckConstraint(
            "expires_at > checked_at",
            name="ck_prod_provider_health_expiry",
        ),
        Index(
            "ix_prod_provider_health_connection_checked",
            "connection_id",
            "checked_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    connection_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    connection_revision: Mapped[int] = mapped_column(Integer)
    credential_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    probe_type: Mapped[str] = mapped_column(String(40))
    status: Mapped[str] = mapped_column(String(20))
    latency_ms: Mapped[int | None] = mapped_column(Integer, nullable=True)
    safe_error_code: Mapped[str | None] = mapped_column(String(80), nullable=True)
    checked_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    checked_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    expires_at: Mapped[datetime] = mapped_column(DateTime(timezone=True))
