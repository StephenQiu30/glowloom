"""Add narrative unit tables and backfill published script versions.

Revision ID: 2b7e4c9a1d63
Revises: 9a4d6e2f1b73
Create Date: 2026-08-13 13:53:44.090473

"""

import hashlib
import json
import re
from collections.abc import Sequence
from datetime import UTC, datetime
from typing import Any
from uuid import UUID, uuid4

import sqlalchemy as sa
from sqlalchemy.dialects import postgresql

from alembic import op

revision: str = "2b7e4c9a1d63"
down_revision: str | Sequence[str] | None = "9a4d6e2f1b73"
branch_labels: str | Sequence[str] | None = None
depends_on: str | Sequence[str] | None = None

_EPISODE_MARKER = re.compile(
    r"^(?:第[一二三四五六七八九十百零〇两\d]+(?:集|话|章)|EP(?:ISODE)?\s*\d+)\s*$",
    re.IGNORECASE,
)
_TITLE = re.compile(r"^[《【].+[》】]$")
_SCENE_HEADING = re.compile(
    r"^(?:内景|外景|内/外|外/内|INT\.?|EXT\.?|INT\.?/EXT\.?)\s*[·.、\- ]",
    re.IGNORECASE,
)
_SPEAKER_PREFIX = re.compile(r"^[^：:\n]{1,30}[：:]")
_NARRATION_PREFIX = re.compile(r"^(?:旁白|系统播报|画外音|内心独白)[：:]")


def _canonical_hash(payload: object) -> str:
    value = json.dumps(
        payload,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return hashlib.sha256(value.encode()).hexdigest()


def _kind(text: str) -> str | None:
    if _EPISODE_MARKER.fullmatch(text) or _TITLE.fullmatch(text):
        return None
    if _SCENE_HEADING.match(text):
        return "scene_heading"
    if _NARRATION_PREFIX.match(text):
        return "narration"
    if _SPEAKER_PREFIX.match(text):
        return "dialogue"
    return "action"


def _parse_units(body: str) -> list[dict[str, Any]]:
    units: list[dict[str, Any]] = []
    offset = 0
    for line in body.splitlines(keepends=True):
        content = line.rstrip("\r\n")
        leading = len(content) - len(content.lstrip())
        trailing = len(content.rstrip())
        exact_text = content[leading:trailing]
        kind = _kind(exact_text) if exact_text else None
        if kind is not None:
            units.append(
                {
                    "kind": kind,
                    "source_start": offset + leading,
                    "source_end": offset + trailing,
                    "exact_text": exact_text,
                }
            )
        offset += len(line)
    return units


def _backfill_published_versions() -> None:
    bind = op.get_bind()
    rows = bind.execute(
        sa.text(
            """
            SELECT v.id, v.workspace_id, s.episode_id, v.body,
                   v.content_hash, v.created_by, v.created_at
            FROM scr_script_versions AS v
            JOIN scr_script_sources AS s ON s.id = v.source_id
            WHERE v.status = 'published'
            ORDER BY v.created_at, v.id
            """
        )
    ).mappings()
    now = datetime.now(UTC)
    for row in rows:
        parsed_units = _parse_units(row["body"])
        if not parsed_units:
            continue
        structure_id = uuid4()
        structure_units: list[dict[str, object]] = []
        unit_rows: list[dict[str, object]] = []
        version_rows: list[dict[str, object]] = []
        unit_version_ids: list[UUID] = []
        for position, parsed in enumerate(parsed_units, start=1):
            unit_id = uuid4()
            unit_version_id = uuid4()
            unit_version_ids.append(unit_version_id)
            source_start = int(parsed["source_start"])
            source_end = int(parsed["source_end"])
            exact_text = str(parsed["exact_text"])
            prefix_text = row["body"][max(0, source_start - 60) : source_start]
            suffix_text = row["body"][source_end : min(len(row["body"]), source_end + 60)]
            unit_rows.append(
                {
                    "id": unit_id,
                    "workspace_id": row["workspace_id"],
                    "episode_id": row["episode_id"],
                    "kind": parsed["kind"],
                    "status": "active",
                    "current_version_id": unit_version_id,
                    "revision": 1,
                    "created_by": row["created_by"],
                    "created_at": row["created_at"] or now,
                    "updated_at": row["created_at"] or now,
                }
            )
            version_rows.append(
                {
                    "id": unit_version_id,
                    "workspace_id": row["workspace_id"],
                    "episode_id": row["episode_id"],
                    "structure_id": structure_id,
                    "script_version_id": row["id"],
                    "unit_id": unit_id,
                    "version_no": 1,
                    "structure_revision": 1,
                    "position": position,
                    "source_start": source_start,
                    "source_end": source_end,
                    "exact_text": exact_text,
                    "text_hash": hashlib.sha256(exact_text.encode()).hexdigest(),
                    "prefix_text": prefix_text,
                    "suffix_text": suffix_text,
                    "required_for_coverage": True,
                    "payload": {},
                    "source_scene_id": None,
                    "source_dialogue_id": None,
                    "origin": "deterministic",
                    "created_by": row["created_by"],
                    "created_at": row["created_at"] or now,
                }
            )
            structure_units.append(
                {
                    "unit_id": str(unit_id),
                    "kind": parsed["kind"],
                    "position": position,
                    "source_start": source_start,
                    "source_end": source_end,
                    "exact_text_hash": hashlib.sha256(exact_text.encode()).hexdigest(),
                    "required_for_coverage": True,
                }
            )
        structure_hash = _canonical_hash(
            {
                "script_version_id": str(row["id"]),
                "revision": 1,
                "units": structure_units,
            }
        )
        dependency_hash = _canonical_hash(
            {
                "script_version_id": str(row["id"]),
                "structure_hash": structure_hash,
                "unit_version_ids": [str(item) for item in unit_version_ids],
            }
        )
        bind.execute(
            sa.text(
                """
                INSERT INTO scr_narrative_structures (
                    id, workspace_id, episode_id, script_version_id, input_hash,
                    parser_version, structure_hash, dependency_hash, revision,
                    command_receipts, created_by, created_at, updated_at
                ) VALUES (
                    :id, :workspace_id, :episode_id, :script_version_id, :input_hash,
                    :parser_version, :structure_hash, :dependency_hash, 1,
                    CAST(:command_receipts AS jsonb), :created_by, :created_at, :updated_at
                )
                """
            ),
            {
                "id": structure_id,
                "workspace_id": row["workspace_id"],
                "episode_id": row["episode_id"],
                "script_version_id": row["id"],
                "input_hash": row["content_hash"],
                "parser_version": "deterministic-lines-v1",
                "structure_hash": structure_hash,
                "dependency_hash": dependency_hash,
                "command_receipts": "{}",
                "created_by": row["created_by"],
                "created_at": row["created_at"] or now,
                "updated_at": row["created_at"] or now,
            },
        )
        bind.execute(
            sa.text(
                """
                INSERT INTO scr_narrative_units (
                    id, workspace_id, episode_id, kind, status, current_version_id,
                    revision, created_by, created_at, updated_at
                ) VALUES (
                    :id, :workspace_id, :episode_id, :kind, :status, :current_version_id,
                    :revision, :created_by, :created_at, :updated_at
                )
                """
            ),
            unit_rows,
        )
        bind.execute(
            sa.text(
                """
                INSERT INTO scr_narrative_unit_versions (
                    id, workspace_id, episode_id, structure_id, script_version_id,
                    unit_id, version_no, structure_revision, position, source_start,
                    source_end, exact_text, text_hash, prefix_text, suffix_text,
                    required_for_coverage, payload, source_scene_id,
                    source_dialogue_id, origin, created_by, created_at
                ) VALUES (
                    :id, :workspace_id, :episode_id, :structure_id, :script_version_id,
                    :unit_id, :version_no, :structure_revision, :position, :source_start,
                    :source_end, :exact_text, :text_hash, :prefix_text, :suffix_text,
                    :required_for_coverage, CAST(:payload AS jsonb), :source_scene_id,
                    :source_dialogue_id, :origin, :created_by, :created_at
                )
                """
            ),
            [{**item, "payload": "{}"} for item in version_rows],
        )


def upgrade() -> None:
    # ### commands auto generated by Alembic - please adjust! ###
    op.create_table(
        "scr_narrative_units",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("kind", sa.String(length=30), nullable=False),
        sa.Column("status", sa.String(length=20), nullable=False),
        sa.Column("current_version_id", sa.Uuid(), nullable=True),
        sa.Column("revision", sa.Integer(), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "kind IN ('scene_heading', 'action', 'dialogue', 'narration')",
            name="ck_scr_narrative_unit_kind",
        ),
        sa.CheckConstraint("status IN ('active', 'retired')", name="ck_scr_narrative_unit_status"),
        sa.CheckConstraint("revision >= 1", name="ck_scr_narrative_unit_revision"),
        sa.ForeignKeyConstraint(
            ["created_by"],
            ["idn_user_accounts.id"],
        ),
        sa.ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_scr_narrative_unit_episode_workspace",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("id", "episode_id", "workspace_id", name="uq_scr_narrative_unit_scope"),
        sa.UniqueConstraint("id", "workspace_id", name="uq_scr_narrative_unit_id_workspace"),
    )
    op.create_index(
        "ix_scr_narrative_unit_episode_status",
        "scr_narrative_units",
        ["episode_id", "status"],
        unique=False,
    )
    op.create_table(
        "scr_narrative_impacts",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("sequence", sa.Integer(), nullable=False),
        sa.Column("trigger", sa.String(length=30), nullable=False),
        sa.Column("episode_revision", sa.Integer(), nullable=False),
        sa.Column("previous_script_version_id", sa.Uuid(), nullable=True),
        sa.Column("current_script_version_id", sa.Uuid(), nullable=False),
        sa.Column("previous_structure_hash", sa.String(length=64), nullable=True),
        sa.Column("current_structure_hash", sa.String(length=64), nullable=False),
        sa.Column("previous_dependency_hash", sa.String(length=64), nullable=True),
        sa.Column("current_dependency_hash", sa.String(length=64), nullable=False),
        sa.Column("previous_unit_count", sa.Integer(), nullable=False),
        sa.Column("current_unit_count", sa.Integer(), nullable=False),
        sa.Column("affected_shot_ids", postgresql.ARRAY(sa.Uuid()), nullable=False),
        sa.Column("invalidated_scopes", postgresql.ARRAY(sa.String(length=40)), nullable=False),
        sa.Column("impact_hash", sa.String(length=64), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "trigger IN ('current_changed', 'structure_corrected')",
            name="ck_scr_narrative_impact_trigger",
        ),
        sa.CheckConstraint(
            "episode_revision >= 1", name="ck_scr_narrative_impact_episode_revision"
        ),
        sa.CheckConstraint(
            "previous_unit_count >= 0 AND current_unit_count >= 0",
            name="ck_scr_narrative_impact_unit_counts",
        ),
        sa.CheckConstraint("sequence >= 1", name="ck_scr_narrative_impact_sequence"),
        sa.ForeignKeyConstraint(
            ["created_by"],
            ["idn_user_accounts.id"],
        ),
        sa.ForeignKeyConstraint(
            ["current_script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_narrative_impact_current_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_scr_narrative_impact_episode_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["previous_script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_narrative_impact_previous_workspace",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "episode_id", "sequence", name="uq_scr_narrative_impact_episode_sequence"
        ),
        sa.UniqueConstraint("id", "workspace_id", name="uq_scr_narrative_impact_id_workspace"),
    )
    op.create_index(
        "ix_scr_narrative_impact_episode_created",
        "scr_narrative_impacts",
        ["episode_id", "created_at"],
        unique=False,
    )
    op.create_table(
        "scr_narrative_structures",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("script_version_id", sa.Uuid(), nullable=False),
        sa.Column("input_hash", sa.String(length=64), nullable=False),
        sa.Column("parser_version", sa.String(length=80), nullable=False),
        sa.Column("structure_hash", sa.String(length=64), nullable=False),
        sa.Column("dependency_hash", sa.String(length=64), nullable=False),
        sa.Column("revision", sa.Integer(), nullable=False),
        sa.Column("command_receipts", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.Column("updated_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint("revision >= 1", name="ck_scr_narrative_structure_revision"),
        sa.ForeignKeyConstraint(
            ["created_by"],
            ["idn_user_accounts.id"],
        ),
        sa.ForeignKeyConstraint(
            ["episode_id", "workspace_id"],
            ["prj_episodes.id", "prj_episodes.workspace_id"],
            name="fk_scr_narrative_structure_episode_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_narrative_structure_script_workspace",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint(
            "id",
            "script_version_id",
            "episode_id",
            "workspace_id",
            name="uq_scr_narrative_structure_scope",
        ),
        sa.UniqueConstraint("id", "workspace_id", name="uq_scr_narrative_structure_id_workspace"),
        sa.UniqueConstraint("script_version_id", name="uq_scr_narrative_structure_script"),
    )
    op.create_index(
        "ix_scr_narrative_structure_episode_created",
        "scr_narrative_structures",
        ["episode_id", "created_at"],
        unique=False,
    )
    op.create_table(
        "scr_narrative_unit_versions",
        sa.Column("id", sa.Uuid(), nullable=False),
        sa.Column("workspace_id", sa.Uuid(), nullable=False),
        sa.Column("episode_id", sa.Uuid(), nullable=False),
        sa.Column("structure_id", sa.Uuid(), nullable=False),
        sa.Column("script_version_id", sa.Uuid(), nullable=False),
        sa.Column("unit_id", sa.Uuid(), nullable=False),
        sa.Column("version_no", sa.Integer(), nullable=False),
        sa.Column("structure_revision", sa.Integer(), nullable=False),
        sa.Column("position", sa.Integer(), nullable=False),
        sa.Column("source_start", sa.Integer(), nullable=False),
        sa.Column("source_end", sa.Integer(), nullable=False),
        sa.Column("exact_text", sa.Text(), nullable=False),
        sa.Column("text_hash", sa.String(length=64), nullable=False),
        sa.Column("prefix_text", sa.String(length=120), nullable=False),
        sa.Column("suffix_text", sa.String(length=120), nullable=False),
        sa.Column("required_for_coverage", sa.Boolean(), nullable=False),
        sa.Column("payload", postgresql.JSONB(astext_type=sa.Text()), nullable=False),
        sa.Column("source_scene_id", sa.Uuid(), nullable=True),
        sa.Column("source_dialogue_id", sa.Uuid(), nullable=True),
        sa.Column("origin", sa.String(length=30), nullable=False),
        sa.Column("created_by", sa.Uuid(), nullable=False),
        sa.Column("created_at", sa.DateTime(timezone=True), nullable=False),
        sa.CheckConstraint(
            "origin IN ('deterministic', 'manual')", name="ck_scr_narrative_version_origin"
        ),
        sa.CheckConstraint("position >= 1", name="ck_scr_narrative_version_position"),
        sa.CheckConstraint("source_end > source_start", name="ck_scr_narrative_version_range"),
        sa.CheckConstraint("source_start >= 0", name="ck_scr_narrative_version_start"),
        sa.CheckConstraint(
            "structure_revision >= 1", name="ck_scr_narrative_version_structure_revision"
        ),
        sa.CheckConstraint("version_no >= 1", name="ck_scr_narrative_version_number"),
        sa.ForeignKeyConstraint(
            ["created_by"],
            ["idn_user_accounts.id"],
        ),
        sa.ForeignKeyConstraint(
            ["script_version_id", "workspace_id"],
            ["scr_script_versions.id", "scr_script_versions.workspace_id"],
            name="fk_scr_narrative_version_script_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["source_dialogue_id", "workspace_id"],
            ["scr_dialogues.id", "scr_dialogues.workspace_id"],
            name="fk_scr_narrative_version_dialogue_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["source_scene_id", "workspace_id"],
            ["scr_scenes.id", "scr_scenes.workspace_id"],
            name="fk_scr_narrative_version_scene_workspace",
        ),
        sa.ForeignKeyConstraint(
            ["structure_id", "script_version_id", "episode_id", "workspace_id"],
            [
                "scr_narrative_structures.id",
                "scr_narrative_structures.script_version_id",
                "scr_narrative_structures.episode_id",
                "scr_narrative_structures.workspace_id",
            ],
            name="fk_scr_narrative_version_structure_scope",
            ondelete="CASCADE",
        ),
        sa.ForeignKeyConstraint(
            ["unit_id", "episode_id", "workspace_id"],
            [
                "scr_narrative_units.id",
                "scr_narrative_units.episode_id",
                "scr_narrative_units.workspace_id",
            ],
            name="fk_scr_narrative_version_unit_scope",
        ),
        sa.PrimaryKeyConstraint("id"),
        sa.UniqueConstraint("id", "workspace_id", name="uq_scr_narrative_version_id_workspace"),
        sa.UniqueConstraint(
            "structure_id",
            "structure_revision",
            "position",
            name="uq_scr_narrative_version_structure_position",
        ),
        sa.UniqueConstraint(
            "structure_id",
            "structure_revision",
            "unit_id",
            name="uq_scr_narrative_version_structure_unit",
        ),
        sa.UniqueConstraint("unit_id", "version_no", name="uq_scr_narrative_version_number"),
    )
    op.create_index(
        "ix_scr_narrative_version_script_range",
        "scr_narrative_unit_versions",
        ["script_version_id", "source_start"],
        unique=False,
    )
    op.create_index(
        "ix_scr_narrative_version_unit_created",
        "scr_narrative_unit_versions",
        ["unit_id", "created_at"],
        unique=False,
    )
    _backfill_published_versions()
    op.create_foreign_key(
        "fk_scr_narrative_unit_current_workspace",
        "scr_narrative_units",
        "scr_narrative_unit_versions",
        ["current_version_id", "workspace_id"],
        ["id", "workspace_id"],
        deferrable=True,
        initially="DEFERRED",
    )
    # ### end Alembic commands ###


def downgrade() -> None:
    # ### commands auto generated by Alembic - please adjust! ###
    op.drop_constraint(
        "fk_scr_narrative_unit_current_workspace",
        "scr_narrative_units",
        type_="foreignkey",
    )
    op.drop_index("ix_scr_narrative_version_unit_created", table_name="scr_narrative_unit_versions")
    op.drop_index("ix_scr_narrative_version_script_range", table_name="scr_narrative_unit_versions")
    op.drop_table("scr_narrative_unit_versions")
    op.drop_index(
        "ix_scr_narrative_structure_episode_created", table_name="scr_narrative_structures"
    )
    op.drop_table("scr_narrative_structures")
    op.drop_index("ix_scr_narrative_impact_episode_created", table_name="scr_narrative_impacts")
    op.drop_table("scr_narrative_impacts")
    op.drop_index("ix_scr_narrative_unit_episode_status", table_name="scr_narrative_units")
    op.drop_table("scr_narrative_units")
    # ### end Alembic commands ###
