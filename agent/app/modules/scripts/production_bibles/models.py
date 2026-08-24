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
    text,
)
from sqlalchemy.dialects.postgresql import ARRAY, JSONB
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class ProductionBible(Base):
    __tablename__ = "scr_production_bibles"
    __table_args__ = (
        ForeignKeyConstraint(
            ("project_id", "workspace_id"),
            ("prj_projects.id", "prj_projects.workspace_id"),
            name="fk_scr_prod_bible_project_workspace",
        ),
        ForeignKeyConstraint(
            ("document_revision_id", "workspace_id"),
            ("scr_document_revisions.id", "scr_document_revisions.workspace_id"),
            name="fk_scr_prod_bible_document_revision_workspace",
        ),
        ForeignKeyConstraint(
            ("task_id", "workspace_id"),
            ("prod_tasks.id", "prod_tasks.workspace_id"),
            name="fk_scr_prod_bible_task_workspace",
        ),
        CheckConstraint(
            "status IN ('queued', 'running', 'needs_review', 'confirmed', "
            "'failed', 'unknown', 'superseded', 'cancelled')",
            name="ck_scr_prod_bible_status",
        ),
        CheckConstraint("char_length(input_hash) = 64", name="ck_scr_prod_bible_input_hash"),
        CheckConstraint(
            "result_hash IS NULL OR char_length(result_hash) = 64",
            name="ck_scr_prod_bible_result_hash",
        ),
        CheckConstraint("checkpoint_revision >= 0", name="ck_scr_prod_bible_checkpoint_revision"),
        CheckConstraint("revision >= 1", name="ck_scr_prod_bible_revision"),
        CheckConstraint(
            "(checkpoint IS NULL AND checkpoint_revision = 0 "
            "AND checkpoint_updated_at IS NULL) OR "
            "(checkpoint IS NOT NULL AND checkpoint_revision >= 1 "
            "AND checkpoint_updated_at IS NOT NULL)",
            name="ck_scr_prod_bible_checkpoint",
        ),
        CheckConstraint(
            "(run_token IS NULL AND lease_expires_at IS NULL) OR "
            "(run_token IS NOT NULL AND lease_expires_at IS NOT NULL)",
            name="ck_scr_prod_bible_lease",
        ),
        CheckConstraint(
            "jsonb_typeof(resume_receipts) = 'object'",
            name="ck_scr_prod_bible_resume_receipts",
        ),
        CheckConstraint(
            "jsonb_typeof(review_receipts) = 'object'",
            name="ck_scr_prod_bible_review_receipts",
        ),
        CheckConstraint(
            "(confirmed_at IS NULL AND confirmed_by IS NULL "
            "AND confirm_idempotency_key IS NULL AND confirm_command_hash IS NULL) OR "
            "(confirmed_at IS NOT NULL AND confirmed_by IS NOT NULL "
            "AND confirm_idempotency_key IS NOT NULL "
            "AND confirm_command_hash IS NOT NULL "
            "AND char_length(confirm_command_hash) = 64)",
            name="ck_scr_prod_bible_confirmation_receipt",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_scr_prod_bible_id_workspace"),
        UniqueConstraint(
            "id",
            "project_id",
            "workspace_id",
            name="uq_scr_prod_bible_scope",
        ),
        UniqueConstraint(
            "document_revision_id",
            "idempotency_key",
            name="uq_scr_prod_bible_revision_idempotency",
        ),
        UniqueConstraint(
            "workspace_id",
            "confirm_idempotency_key",
            name="uq_scr_prod_bible_confirm_idempotency",
        ),
        Index(
            "uq_scr_prod_bible_project_confirmed",
            "project_id",
            unique=True,
            postgresql_where=text("status = 'confirmed'"),
        ),
        Index(
            "ix_scr_prod_bible_project_status_created",
            "project_id",
            "status",
            "created_at",
        ),
        Index(
            "ix_scr_prod_bible_revision_status_created",
            "document_revision_id",
            "status",
            "created_at",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    document_revision_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    task_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    status: Mapped[str] = mapped_column(String(30), default="queued")
    input_hash: Mapped[str] = mapped_column(String(64))
    result_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    engine_version: Mapped[str] = mapped_column(String(80))
    model_name: Mapped[str] = mapped_column(String(160))
    prompt_version: Mapped[str] = mapped_column(String(80))
    schema_version: Mapped[str] = mapped_column(String(80))
    harness_version: Mapped[str] = mapped_column(String(80))
    checkpoint: Mapped[dict[str, Any] | None] = mapped_column(
        JSONB(none_as_null=True), nullable=True, default=None
    )
    checkpoint_revision: Mapped[int] = mapped_column(Integer, default=0)
    checkpoint_updated_at: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True, default=None
    )
    run_token: Mapped[UUID | None] = mapped_column(Uuid, nullable=True, default=None)
    lease_expires_at: Mapped[datetime | None] = mapped_column(
        DateTime(timezone=True), nullable=True, default=None
    )
    review_issues: Mapped[list[dict[str, Any]]] = mapped_column(JSONB, default=list)
    resume_receipts: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    review_receipts: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    idempotency_key: Mapped[str] = mapped_column(String(200))
    confirm_idempotency_key: Mapped[str | None] = mapped_column(String(200), nullable=True)
    confirm_command_hash: Mapped[str | None] = mapped_column(String(64), nullable=True)
    confirm_result: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    confirmed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    confirmed_by: Mapped[UUID | None] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=True
    )
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class ProductionBibleEntity(Base):
    __tablename__ = "scr_production_bible_entities"
    __table_args__ = (
        ForeignKeyConstraint(
            ("bible_id", "project_id", "workspace_id"),
            (
                "scr_production_bibles.id",
                "scr_production_bibles.project_id",
                "scr_production_bibles.workspace_id",
            ),
            name="fk_scr_prod_bible_entity_bible_scope",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ("asset_id", "workspace_id"),
            ("ast_assets.id", "ast_assets.workspace_id"),
            name="fk_scr_prod_bible_entity_asset_workspace",
        ),
        CheckConstraint(
            "kind IN ('character', 'location', 'prop', 'costume', 'visual_style', 'voice')",
            name="ck_scr_prod_bible_entity_kind",
        ),
        CheckConstraint(
            "jsonb_typeof(stable_spec) = 'object'",
            name="ck_scr_prod_bible_entity_stable_spec",
        ),
        CheckConstraint(
            "jsonb_typeof(evidence) = 'array'",
            name="ck_scr_prod_bible_entity_evidence",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_scr_prod_bible_entity_id_workspace"),
        UniqueConstraint(
            "id",
            "bible_id",
            "project_id",
            "workspace_id",
            name="uq_scr_prod_bible_entity_scope",
        ),
        UniqueConstraint(
            "bible_id",
            "entity_key",
            name="uq_scr_prod_bible_entity_key",
        ),
        UniqueConstraint(
            "bible_id",
            "asset_id",
            name="uq_scr_prod_bible_entity_asset",
        ),
        Index(
            "ix_scr_prod_bible_entity_name",
            "bible_id",
            "kind",
            "normalized_name",
        ),
        Index("ix_scr_prod_bible_entity_asset", "asset_id"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    bible_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    entity_key: Mapped[str] = mapped_column(String(100))
    kind: Mapped[str] = mapped_column(String(30))
    canonical_name: Mapped[str] = mapped_column(String(200))
    normalized_name: Mapped[str] = mapped_column(String(200))
    aliases: Mapped[list[str]] = mapped_column(ARRAY(Text), default=list)
    stable_spec: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    episode_numbers: Mapped[list[int]] = mapped_column(ARRAY(Integer), default=list)
    evidence: Mapped[list[dict[str, Any]]] = mapped_column(JSONB, default=list)
    asset_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class ProductionBibleEntityState(Base):
    __tablename__ = "scr_production_bible_entity_states"
    __table_args__ = (
        ForeignKeyConstraint(
            ("entity_id", "bible_id", "project_id", "workspace_id"),
            (
                "scr_production_bible_entities.id",
                "scr_production_bible_entities.bible_id",
                "scr_production_bible_entities.project_id",
                "scr_production_bible_entities.workspace_id",
            ),
            name="fk_scr_prod_bible_state_entity_scope",
            ondelete="CASCADE",
        ),
        ForeignKeyConstraint(
            ("asset_state_id", "workspace_id"),
            ("ast_asset_states.id", "ast_asset_states.workspace_id"),
            name="fk_scr_prod_bible_state_asset_state_workspace",
        ),
        ForeignKeyConstraint(
            ("asset_version_id", "workspace_id"),
            ("ast_asset_versions.id", "ast_asset_versions.workspace_id"),
            name="fk_scr_prod_bible_state_asset_version_workspace",
        ),
        CheckConstraint(
            "jsonb_typeof(state_spec) = 'object'",
            name="ck_scr_prod_bible_state_spec",
        ),
        CheckConstraint(
            "jsonb_typeof(evidence) = 'array'",
            name="ck_scr_prod_bible_state_evidence",
        ),
        CheckConstraint(
            "(asset_state_id IS NULL AND asset_version_id IS NULL) OR "
            "(asset_state_id IS NOT NULL AND asset_version_id IS NOT NULL)",
            name="ck_scr_prod_bible_state_materialization",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_scr_prod_bible_state_id_workspace"),
        UniqueConstraint(
            "entity_id",
            "state_key",
            name="uq_scr_prod_bible_state_key",
        ),
        UniqueConstraint(
            "bible_id",
            "asset_state_id",
            name="uq_scr_prod_bible_state_asset_state",
        ),
        UniqueConstraint(
            "bible_id",
            "asset_version_id",
            name="uq_scr_prod_bible_state_asset_version",
        ),
        Index("ix_scr_prod_bible_state_bible_entity", "bible_id", "entity_id"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    bible_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    entity_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    state_key: Mapped[str] = mapped_column(String(80))
    label: Mapped[str] = mapped_column(String(120))
    state_spec: Mapped[dict[str, Any]] = mapped_column(JSONB, default=dict)
    episode_numbers: Mapped[list[int]] = mapped_column(ARRAY(Integer), default=list)
    evidence: Mapped[list[dict[str, Any]]] = mapped_column(JSONB, default=list)
    asset_state_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    asset_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class ProductionBibleWorldEntry(Base):
    __tablename__ = "scr_production_bible_world_entries"
    __table_args__ = (
        ForeignKeyConstraint(
            ("bible_id", "project_id", "workspace_id"),
            (
                "scr_production_bibles.id",
                "scr_production_bibles.project_id",
                "scr_production_bibles.workspace_id",
            ),
            name="fk_scr_prod_bible_world_bible_scope",
            ondelete="CASCADE",
        ),
        CheckConstraint(
            "char_length(trim(category)) >= 1", name="ck_scr_prod_bible_world_category"
        ),
        CheckConstraint(
            "jsonb_typeof(evidence) = 'array'",
            name="ck_scr_prod_bible_world_evidence",
        ),
        CheckConstraint(
            "cardinality(facts) >= 1 OR cardinality(rules) >= 1",
            name="ck_scr_prod_bible_world_content",
        ),
        UniqueConstraint("id", "workspace_id", name="uq_scr_prod_bible_world_id_workspace"),
        UniqueConstraint(
            "bible_id",
            "entry_key",
            name="uq_scr_prod_bible_world_key",
        ),
        Index("ix_scr_prod_bible_world_category", "bible_id", "category"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    project_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    bible_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    entry_key: Mapped[str] = mapped_column(String(100))
    category: Mapped[str] = mapped_column(String(80))
    title: Mapped[str] = mapped_column(String(200))
    facts: Mapped[list[str]] = mapped_column(ARRAY(Text), default=list)
    rules: Mapped[list[str]] = mapped_column(ARRAY(Text), default=list)
    entity_keys: Mapped[list[str]] = mapped_column(ARRAY(String(100)), default=list)
    episode_numbers: Mapped[list[int]] = mapped_column(ARRAY(Integer), default=list)
    evidence: Mapped[list[dict[str, Any]]] = mapped_column(JSONB, default=list)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )
