"""Add storyboard narrative coverage.

Revision ID: 7a2d9c4e6f10
Revises: ecdbb9f876f8
Create Date: 2026-08-13 23:10:00

"""

from collections.abc import Sequence

import sqlalchemy as sa

from alembic import op

revision: str = "7a2d9c4e6f10"
down_revision: str | Sequence[str] | None = "ecdbb9f876f8"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None


def upgrade() -> None:
    op.create_unique_constraint(
        "uq_sbd_shot_episode_scope",
        "sbd_shots",
        ["id", "episode_id", "workspace_id"],
    )
    op.create_unique_constraint(
        "uq_sbd_spec_shot_scope",
        "sbd_shot_spec_versions",
        ["id", "shot_id", "workspace_id"],
    )
    op.create_table(
        "sbd_narrative_references",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("shot_id", sa.Uuid(), nullable=False),
        sa.Column("shot_spec_version_id", sa.Uuid(), nullable=False),
        sa.Column("narrative_unit_id", sa.Uuid(), nullable=False),
        sa.Column("unit_version_id", sa.Uuid(), nullable=False),
        sa.Column("channel", sa.String(length=20), nullable=False),
        sa.Column("role", sa.String(length=30), nullable=False),
        sa.Column("coverage_mode", sa.String(length=20), nullable=False),
        sa.Column("segment_start", sa.Integer(), nullable=True),
        sa.Column("segment_end", sa.Integer(), nullable=True),
        sa.Column("segment_key", sa.String(length=50), nullable=False),
        sa.Column("contribution", sa.String(length=20), nullable=False),
        sa.Column("origin", sa.String(length=20), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "channel IN ('visual', 'audio', 'both')",
            name="ck_sbd_narrative_ref_channel",
        ),
        sa.CheckConstraint(
            "contribution IN ('required', 'supporting')",
            name="ck_sbd_narrative_ref_contribution",
        ),
        sa.CheckConstraint(
            "coverage_mode IN ('full', 'partial')",
            name="ck_sbd_narrative_ref_mode",
        ),
        sa.CheckConstraint(
            "origin IN ('ai', 'human', 'migrated')",
            name="ck_sbd_narrative_ref_origin",
        ),
        sa.CheckConstraint(
            "role IN ('primary', 'dialogue', 'reaction', 'insert', 'setup', "
            "'payoff', 'transition', 'supporting')",
            name="ck_sbd_narrative_ref_role",
        ),
        sa.CheckConstraint(
            "(coverage_mode = 'full' AND segment_start IS NULL AND segment_end IS NULL "
            "AND segment_key = 'full') OR "
            "(coverage_mode = 'partial' AND segment_start >= 0 "
            "AND segment_end > segment_start AND segment_key <> 'full')",
            name="ck_sbd_narrative_ref_segment",
        ),
        sa.ForeignKeyConstraint(["created_by"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(
            ["shot_id", "episode_id", "workspace_id"],
            ["sbd_shots.id", "sbd_shots.episode_id", "sbd_shots.workspace_id"],
            name="fk_sbd_narrative_ref_shot_scope",
        ),
        sa.ForeignKeyConstraint(
            ["shot_spec_version_id", "shot_id", "workspace_id"],
            [
                "sbd_shot_spec_versions.id",
                "sbd_shot_spec_versions.shot_id",
                "sbd_shot_spec_versions.workspace_id",
            ],
            name="fk_sbd_narrative_ref_spec_scope",
        ),
        sa.ForeignKeyConstraint(
            ["unit_version_id", "narrative_unit_id", "episode_id", "workspace_id"],
            [
                "scr_narrative_unit_versions.id",
                "scr_narrative_unit_versions.unit_id",
                "scr_narrative_unit_versions.episode_id",
                "scr_narrative_unit_versions.workspace_id",
            ],
            name="fk_sbd_narrative_ref_unit_scope",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "shot_spec_version_id",
            "unit_version_id",
            "channel",
            "segment_key",
            name="uq_sbd_narrative_ref_edge",
        ),
    )
    op.create_index(
        "ix_sbd_narrative_ref_episode",
        "sbd_narrative_references",
        ["episode_id"],
    )
    op.create_index(
        "ix_sbd_narrative_ref_spec",
        "sbd_narrative_references",
        ["shot_spec_version_id"],
    )
    op.create_index(
        "ix_sbd_narrative_ref_unit",
        "sbd_narrative_references",
        ["unit_version_id"],
    )
    op.create_table(
        "sbd_coverage_decisions",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("sequence", sa.Integer(), nullable=False),
        sa.Column("action", sa.String(length=30), nullable=False),
        sa.Column("narrative_unit_id", sa.Uuid(), nullable=True),
        sa.Column("unit_version_id", sa.Uuid(), nullable=True),
        sa.Column("shot_id", sa.Uuid(), nullable=True),
        sa.Column("shot_spec_version_id", sa.Uuid(), nullable=True),
        sa.Column("basis_hash", sa.String(length=64), nullable=False),
        sa.Column("reason", sa.String(length=1000), nullable=False),
        sa.Column("evidence", sa.Text(), nullable=True),
        sa.Column("command_hash", sa.String(length=64), nullable=False),
        sa.Column("idempotency_key", sa.String(length=200), nullable=False),
        sa.Column("actor_id", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "action IN ('approve_omission', 'revoke_omission', "
            "'approve_invented', 'revoke_invented')",
            name="ck_sbd_coverage_decision_action",
        ),
        sa.CheckConstraint(
            "sequence >= 1",
            name="ck_sbd_coverage_decision_sequence",
        ),
        sa.CheckConstraint(
            "((action IN ('approve_omission', 'revoke_omission')) "
            "AND unit_version_id IS NOT NULL AND narrative_unit_id IS NOT NULL "
            "AND shot_spec_version_id IS NULL AND shot_id IS NULL) OR "
            "((action IN ('approve_invented', 'revoke_invented')) "
            "AND shot_spec_version_id IS NOT NULL AND shot_id IS NOT NULL "
            "AND unit_version_id IS NULL AND narrative_unit_id IS NULL)",
            name="ck_sbd_coverage_decision_target",
        ),
        sa.ForeignKeyConstraint(["actor_id"], ["idn_user_accounts.id"]),
        sa.ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_sbd_coverage_decision_episode",
        ),
        sa.ForeignKeyConstraint(
            ["shot_id", "episode_id", "workspace_id"],
            ["sbd_shots.id", "sbd_shots.episode_id", "sbd_shots.workspace_id"],
            name="fk_sbd_coverage_decision_shot",
        ),
        sa.ForeignKeyConstraint(
            ["shot_spec_version_id", "shot_id", "workspace_id"],
            [
                "sbd_shot_spec_versions.id",
                "sbd_shot_spec_versions.shot_id",
                "sbd_shot_spec_versions.workspace_id",
            ],
            name="fk_sbd_coverage_decision_spec",
        ),
        sa.ForeignKeyConstraint(
            ["unit_version_id", "narrative_unit_id", "episode_id", "workspace_id"],
            [
                "scr_narrative_unit_versions.id",
                "scr_narrative_unit_versions.unit_id",
                "scr_narrative_unit_versions.episode_id",
                "scr_narrative_unit_versions.workspace_id",
            ],
            name="fk_sbd_coverage_decision_unit",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "workspace_id",
            "idempotency_key",
            name="uq_sbd_coverage_decision_idempotency",
        ),
        sa.UniqueConstraint(
            "episode_id",
            "sequence",
            name="uq_sbd_coverage_decision_sequence",
        ),
    )
    op.create_index(
        "ix_sbd_coverage_decision_episode_created",
        "sbd_coverage_decisions",
        ["episode_id", "sequence"],
    )


def downgrade() -> None:
    op.drop_index(
        "ix_sbd_coverage_decision_episode_created",
        table_name="sbd_coverage_decisions",
    )
    op.drop_table("sbd_coverage_decisions")
    op.drop_index(
        "ix_sbd_narrative_ref_unit",
        table_name="sbd_narrative_references",
    )
    op.drop_index(
        "ix_sbd_narrative_ref_spec",
        table_name="sbd_narrative_references",
    )
    op.drop_index(
        "ix_sbd_narrative_ref_episode",
        table_name="sbd_narrative_references",
    )
    op.drop_table("sbd_narrative_references")
    op.drop_constraint(
        "uq_sbd_spec_shot_scope",
        "sbd_shot_spec_versions",
        type_="unique",
    )
    op.drop_constraint(
        "uq_sbd_shot_episode_scope",
        "sbd_shots",
        type_="unique",
    )
