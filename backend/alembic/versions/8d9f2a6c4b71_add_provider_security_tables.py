"""Add provider security tables.

Revision ID: 8d9f2a6c4b71
Revises: 95c0d24572c5
Create Date: 2026-08-13 10:21:00

Compatibility note: the first Alembic baseline mistakenly included the four tables
owned by this revision. Databases that applied that baseline detect the complete table
set and safely skip creation. Historical pre-Alembic 38-table databases add the table
set here after adoption. A partial table set fails closed instead of continuing from an
unknown intermediate state.
"""

from collections.abc import Sequence

import sqlalchemy as sa
from sqlalchemy import inspect
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "8d9f2a6c4b71"
down_revision: str | Sequence[str] | None = "95c0d24572c5"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

PROVIDER_TABLE_NAMES = {
    "prod_provider_bindings",
    "prod_provider_connections",
    "prod_provider_credential_versions",
    "prod_provider_health_checks",
}
PROVIDER_CAPABILITY_UNIQUE = "uq_prod_capability_id_version"


def _provider_tables_present() -> set[str]:
    existing = set(inspect(op.get_bind()).get_table_names())
    return existing & PROVIDER_TABLE_NAMES


def _provider_capability_unique_present() -> bool:
    constraints = inspect(op.get_bind()).get_unique_constraints("prod_model_capabilities")
    return any(constraint["name"] == PROVIDER_CAPABILITY_UNIQUE for constraint in constraints)


def upgrade() -> None:
    present = _provider_tables_present()
    capability_unique_present = _provider_capability_unique_present()
    if present == PROVIDER_TABLE_NAMES and capability_unique_present:
        return
    if present or capability_unique_present:
        raise RuntimeError(
            "partial Provider schema cannot be upgraded safely; "
            f"tables={sorted(present)!r}, "
            f"capability_unique={capability_unique_present!r}"
        )

    op.create_unique_constraint(
        PROVIDER_CAPABILITY_UNIQUE,
        "prod_model_capabilities",
        ["id", "config_version"],
    )
    op.create_table(
        "prod_provider_connections",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("preset_id", sa.String(length=100), nullable=False),
        sa.Column("catalog_version", sa.Integer(), nullable=False),
        sa.Column("display_name", sa.String(length=200), nullable=False),
        sa.Column("protocol", sa.String(length=40), nullable=False),
        sa.Column("region", sa.String(length=100), nullable=True),
        sa.Column("base_url", sa.String(length=2048), nullable=False),
        sa.Column("non_secret_config", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("configuration_status", sa.String(length=20), nullable=False),
        sa.Column("revision", sa.Integer(), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("updated_by", sa.Uuid(), nullable=False),
        sa.Column("archived_at", sa.DateTime(timezone=True), nullable=True),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "configuration_status IN ('incomplete', 'valid', 'invalid')",
            name="ck_prod_provider_connection_configuration",
        ),
        sa.CheckConstraint(
            "protocol IN ('openai_compatible', 'anthropic_native', "
            "'gemini_native', 'ark_native')",
            name="ck_prod_provider_connection_protocol",
        ),
        sa.CheckConstraint("catalog_version >= 1", name="ck_prod_provider_connection_catalog"),
        sa.CheckConstraint("revision >= 1", name="ck_prod_provider_connection_revision"),
        sa.ForeignKeyConstraint(["created_by"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(["updated_by"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(["workspace_id"], ["idn_workspaces.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("id", "workspace_id", name="uq_prod_provider_connection_id_workspace"),
    )
    op.create_index(
        "ix_prod_provider_connection_workspace_archived",
        "prod_provider_connections",
        ["workspace_id", "archived_at"],
        unique=False,
    )
    op.create_table(
        "prod_provider_credential_versions",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("connection_id", sa.Uuid(), nullable=False),
        sa.Column("version", sa.Integer(), nullable=False),
        sa.Column("key_id", sa.String(length=100), nullable=False),
        sa.Column("nonce", sa.LargeBinary(), nullable=False),
        sa.Column("ciphertext", sa.LargeBinary(), nullable=False),
        sa.Column("auth_tag", sa.LargeBinary(), nullable=False),
        sa.Column("fingerprint_hmac", sa.String(length=64), nullable=False),
        sa.Column("status", sa.String(length=20), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("retired_at", sa.DateTime(timezone=True), nullable=True),
        sa.CheckConstraint(
            "status IN ('current', 'retiring', 'revoked')",
            name="ck_prod_provider_credential_status",
        ),
        sa.CheckConstraint(
            "char_length(fingerprint_hmac) = 64",
            name="ck_prod_provider_credential_fingerprint",
        ),
        sa.CheckConstraint(
            "octet_length(auth_tag) = 16",
            name="ck_prod_provider_credential_auth_tag",
        ),
        sa.CheckConstraint(
            "octet_length(ciphertext) > 0",
            name="ck_prod_provider_credential_ciphertext",
        ),
        sa.CheckConstraint("octet_length(nonce) = 12", name="ck_prod_provider_credential_nonce"),
        sa.CheckConstraint("version >= 1", name="ck_prod_provider_credential_version"),
        sa.ForeignKeyConstraint(
            ["connection_id", "workspace_id"],
            ["prod_provider_connections.id", "prod_provider_connections.workspace_id"],
            name="fk_prod_provider_credential_connection_workspace",
        ),
        sa.ForeignKeyConstraint(["created_by"], ["idn_user_accounts.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "connection_id",
            "fingerprint_hmac",
            name="uq_prod_provider_credential_connection_fingerprint",
        ),
        sa.UniqueConstraint(
            "connection_id",
            "version",
            name="uq_prod_provider_credential_connection_version",
        ),
        sa.UniqueConstraint(
            "id",
            "workspace_id",
            "connection_id",
            name="uq_prod_provider_credential_identity",
        ),
    )
    op.create_index(
        "ix_prod_provider_credential_workspace_created",
        "prod_provider_credential_versions",
        ["workspace_id", "created_at"],
        unique=False,
    )
    op.create_index(
        "uq_prod_provider_credential_current",
        "prod_provider_credential_versions",
        ["connection_id"],
        unique=True,
        postgresql_where=sa.text("status = 'current'"),
    )
    op.create_table(
        "prod_provider_bindings",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("usage_type", sa.String(length=40), nullable=False),
        sa.Column("connection_id", sa.Uuid(), nullable=False),
        sa.Column("credential_version_id", sa.Uuid(), nullable=False),
        sa.Column("capability_id", sa.Uuid(), nullable=False),
        sa.Column("capability_config_version", sa.Integer(), nullable=False),
        sa.Column("binding_revision", sa.Integer(), nullable=False),
        sa.Column("status", sa.String(length=20), nullable=False),
        sa.Column("activated_by", sa.Uuid(), nullable=False),
        sa.Column("activated_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("deactivated_by", sa.Uuid(), nullable=True),
        sa.Column("deactivated_at", sa.DateTime(timezone=True), nullable=True),
        sa.CheckConstraint(
            "(status = 'active' AND deactivated_by IS NULL "
            "AND deactivated_at IS NULL) OR (status = 'inactive' "
            "AND deactivated_by IS NOT NULL AND deactivated_at IS NOT NULL)",
            name="ck_prod_provider_binding_lifecycle",
        ),
        sa.CheckConstraint(
            "status IN ('active', 'inactive')",
            name="ck_prod_provider_binding_status",
        ),
        sa.CheckConstraint(
            "usage_type IN ('script_structure', 'image_generation', 'video_generation')",
            name="ck_prod_provider_binding_usage",
        ),
        sa.CheckConstraint("binding_revision >= 1", name="ck_prod_provider_binding_revision"),
        sa.CheckConstraint(
            "capability_config_version >= 1",
            name="ck_prod_provider_binding_capability_version",
        ),
        sa.ForeignKeyConstraint(["activated_by"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(
            ["capability_id", "capability_config_version"],
            ["prod_model_capabilities.id", "prod_model_capabilities.config_version"],
            name="fk_prod_provider_binding_capability_version",
        ),
        sa.ForeignKeyConstraint(
            ["credential_version_id", "workspace_id", "connection_id"],
            [
                "prod_provider_credential_versions.id",
                "prod_provider_credential_versions.workspace_id",
                "prod_provider_credential_versions.connection_id",
            ],
            name="fk_prod_provider_binding_credential_identity",
        ),
        sa.ForeignKeyConstraint(["deactivated_by"], ["idn_user_accounts.id"]),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("id", "workspace_id", name="uq_prod_provider_binding_id_workspace"),
    )
    op.create_index(
        "ix_prod_provider_binding_connection_status",
        "prod_provider_bindings",
        ["connection_id", "status"],
        unique=False,
    )
    op.create_index(
        "uq_prod_provider_binding_active_usage",
        "prod_provider_bindings",
        ["workspace_id", "usage_type"],
        unique=True,
        postgresql_where=sa.text("status = 'active'"),
    )
    op.create_table(
        "prod_provider_health_checks",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("connection_id", sa.Uuid(), nullable=False),
        sa.Column("connection_revision", sa.Integer(), nullable=False),
        sa.Column("credential_version_id", sa.Uuid(), nullable=False),
        sa.Column("probe_type", sa.String(length=40), nullable=False),
        sa.Column("status", sa.String(length=20), nullable=False),
        sa.Column("latency_ms", sa.Integer(), nullable=True),
        sa.Column("safe_error_code", sa.String(length=80), nullable=True),
        sa.Column("checked_by", sa.Uuid(), nullable=False),
        sa.Column("checked_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("expires_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "probe_type IN ('model_discovery', 'metadata')",
            name="ck_prod_provider_health_probe_type",
        ),
        sa.CheckConstraint(
            "status IN ('healthy', 'degraded', 'unreachable')",
            name="ck_prod_provider_health_status",
        ),
        sa.CheckConstraint(
            "connection_revision >= 1",
            name="ck_prod_provider_health_connection_revision",
        ),
        sa.CheckConstraint("expires_at > checked_at", name="ck_prod_provider_health_expiry"),
        sa.CheckConstraint(
            "latency_ms IS NULL OR latency_ms >= 0",
            name="ck_prod_provider_health_latency",
        ),
        sa.ForeignKeyConstraint(["checked_by"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(
            ["credential_version_id", "workspace_id", "connection_id"],
            [
                "prod_provider_credential_versions.id",
                "prod_provider_credential_versions.workspace_id",
                "prod_provider_credential_versions.connection_id",
            ],
            name="fk_prod_provider_health_credential_identity",
        ),
        sa.PrimaryKeyConstraint("id"),
    )
    op.create_index(
        "ix_prod_provider_health_connection_checked",
        "prod_provider_health_checks",
        ["connection_id", "checked_at"],
        unique=False,
    )


def downgrade() -> None:
    present = _provider_tables_present()
    capability_unique_present = _provider_capability_unique_present()
    if not present and not capability_unique_present:
        return
    if present != PROVIDER_TABLE_NAMES or not capability_unique_present:
        raise RuntimeError(
            "partial Provider schema cannot be downgraded safely; "
            f"tables={sorted(present)!r}, "
            f"capability_unique={capability_unique_present!r}"
        )

    op.drop_index(
        "ix_prod_provider_health_connection_checked",
        table_name="prod_provider_health_checks",
    )
    op.drop_table("prod_provider_health_checks")
    op.drop_index(
        "uq_prod_provider_binding_active_usage",
        table_name="prod_provider_bindings",
        postgresql_where=sa.text("status = 'active'"),
    )
    op.drop_index(
        "ix_prod_provider_binding_connection_status",
        table_name="prod_provider_bindings",
    )
    op.drop_table("prod_provider_bindings")
    op.drop_index(
        "uq_prod_provider_credential_current",
        table_name="prod_provider_credential_versions",
        postgresql_where=sa.text("status = 'current'"),
    )
    op.drop_index(
        "ix_prod_provider_credential_workspace_created",
        table_name="prod_provider_credential_versions",
    )
    op.drop_table("prod_provider_credential_versions")
    op.drop_index(
        "ix_prod_provider_connection_workspace_archived",
        table_name="prod_provider_connections",
    )
    op.drop_table("prod_provider_connections")
    op.drop_constraint(
        PROVIDER_CAPABILITY_UNIQUE,
        "prod_model_capabilities",
        type_="unique",
    )
