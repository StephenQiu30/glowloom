from datetime import UTC, datetime
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
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class ShotNarrativeReference(Base):
    __tablename__ = "sbd_narrative_references"
    __table_args__ = (
        ForeignKeyConstraint(
            ("shot_id", "episode_id", "workspace_id"),
            ("sbd_shots.id", "sbd_shots.episode_id", "sbd_shots.workspace_id"),
            name="fk_sbd_narrative_ref_shot_scope",
        ),
        ForeignKeyConstraint(
            ("shot_spec_version_id", "shot_id", "workspace_id"),
            (
                "sbd_shot_spec_versions.id",
                "sbd_shot_spec_versions.shot_id",
                "sbd_shot_spec_versions.workspace_id",
            ),
            name="fk_sbd_narrative_ref_spec_scope",
        ),
        ForeignKeyConstraint(
            (
                "unit_version_id",
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
            name="fk_sbd_narrative_ref_unit_scope",
        ),
        CheckConstraint(
            "channel IN ('visual', 'audio', 'both')",
            name="ck_sbd_narrative_ref_channel",
        ),
        CheckConstraint(
            "role IN ('primary', 'dialogue', 'reaction', 'insert', 'setup', "
            "'payoff', 'transition', 'supporting')",
            name="ck_sbd_narrative_ref_role",
        ),
        CheckConstraint(
            "coverage_mode IN ('full', 'partial')",
            name="ck_sbd_narrative_ref_mode",
        ),
        CheckConstraint(
            "(coverage_mode = 'full' AND segment_start IS NULL AND segment_end IS NULL "
            "AND segment_key = 'full') OR "
            "(coverage_mode = 'partial' AND segment_start >= 0 "
            "AND segment_end > segment_start AND segment_key <> 'full')",
            name="ck_sbd_narrative_ref_segment",
        ),
        CheckConstraint(
            "contribution IN ('required', 'supporting')",
            name="ck_sbd_narrative_ref_contribution",
        ),
        CheckConstraint(
            "origin IN ('ai', 'human', 'migrated')",
            name="ck_sbd_narrative_ref_origin",
        ),
        UniqueConstraint(
            "shot_spec_version_id",
            "unit_version_id",
            "channel",
            "segment_key",
            name="uq_sbd_narrative_ref_edge",
        ),
        Index("ix_sbd_narrative_ref_unit", "unit_version_id"),
        Index("ix_sbd_narrative_ref_spec", "shot_spec_version_id"),
        Index("ix_sbd_narrative_ref_episode", "episode_id"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    shot_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    shot_spec_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    narrative_unit_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    unit_version_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    channel: Mapped[str] = mapped_column(String(20))
    role: Mapped[str] = mapped_column(String(30))
    coverage_mode: Mapped[str] = mapped_column(String(20))
    segment_start: Mapped[int | None] = mapped_column(Integer, nullable=True)
    segment_end: Mapped[int | None] = mapped_column(Integer, nullable=True)
    segment_key: Mapped[str] = mapped_column(String(50))
    contribution: Mapped[str] = mapped_column(String(20))
    origin: Mapped[str] = mapped_column(String(20))
    created_by: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_user_accounts.id"), nullable=False
    )
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)


class CoverageDecision(Base):
    __tablename__ = "sbd_coverage_decisions"
    __table_args__ = (
        ForeignKeyConstraint(
            ("episode_id", "workspace_id"),
            ("prj_episodes.id", "prj_episodes.workspace_id"),
            name="fk_sbd_coverage_decision_episode",
        ),
        ForeignKeyConstraint(
            (
                "unit_version_id",
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
            name="fk_sbd_coverage_decision_unit",
        ),
        ForeignKeyConstraint(
            ("shot_id", "episode_id", "workspace_id"),
            ("sbd_shots.id", "sbd_shots.episode_id", "sbd_shots.workspace_id"),
            name="fk_sbd_coverage_decision_shot",
        ),
        ForeignKeyConstraint(
            ("shot_spec_version_id", "shot_id", "workspace_id"),
            (
                "sbd_shot_spec_versions.id",
                "sbd_shot_spec_versions.shot_id",
                "sbd_shot_spec_versions.workspace_id",
            ),
            name="fk_sbd_coverage_decision_spec",
        ),
        CheckConstraint("sequence >= 1", name="ck_sbd_coverage_decision_sequence"),
        CheckConstraint(
            "action IN ('approve_omission', 'revoke_omission', "
            "'approve_invented', 'revoke_invented')",
            name="ck_sbd_coverage_decision_action",
        ),
        CheckConstraint(
            "((action IN ('approve_omission', 'revoke_omission')) "
            "AND unit_version_id IS NOT NULL AND narrative_unit_id IS NOT NULL "
            "AND shot_spec_version_id IS NULL AND shot_id IS NULL) OR "
            "((action IN ('approve_invented', 'revoke_invented')) "
            "AND shot_spec_version_id IS NOT NULL AND shot_id IS NOT NULL "
            "AND unit_version_id IS NULL AND narrative_unit_id IS NULL)",
            name="ck_sbd_coverage_decision_target",
        ),
        UniqueConstraint(
            "episode_id",
            "sequence",
            name="uq_sbd_coverage_decision_sequence",
        ),
        UniqueConstraint(
            "workspace_id",
            "idempotency_key",
            name="uq_sbd_coverage_decision_idempotency",
        ),
        Index(
            "ix_sbd_coverage_decision_episode_created",
            "episode_id",
            "sequence",
        ),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    episode_id: Mapped[UUID] = mapped_column(Uuid, nullable=False)
    sequence: Mapped[int] = mapped_column(Integer)
    action: Mapped[str] = mapped_column(String(30))
    narrative_unit_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    unit_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    shot_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    shot_spec_version_id: Mapped[UUID | None] = mapped_column(Uuid, nullable=True)
    basis_hash: Mapped[str] = mapped_column(String(64))
    reason: Mapped[str] = mapped_column(String(1000))
    evidence: Mapped[str | None] = mapped_column(Text, nullable=True)
    command_hash: Mapped[str] = mapped_column(String(64))
    idempotency_key: Mapped[str] = mapped_column(String(200))
    actor_id: Mapped[UUID] = mapped_column(Uuid, ForeignKey("idn_user_accounts.id"), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
