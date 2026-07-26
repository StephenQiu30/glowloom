from __future__ import annotations

import json
from collections.abc import Mapping
from typing import Any

import asyncpg  # type: ignore[import-untyped]

from schemas.subtitle_versions import SubtitleVersionSnapshot
from schemas.subtitles import SubtitleContentV1, SubtitleInputRefsV1


def map_subtitle(row: Mapping[str, Any]) -> SubtitleVersionSnapshot:
    inputs = _json_text(row["input_refs_json"])
    content = json.dumps(
        {"language": row["language"], "cues": _json_value(row["cues_json"])}
    )
    return SubtitleVersionSnapshot(
        id=row["id"],
        episode_id=row["episode_id"],
        version=row["version"],
        parent_id=row["parent_id"],
        script_version_id=row["script_version_id"],
        shot_spec_version_id=row["shot_spec_version_id"],
        input_refs=SubtitleInputRefsV1.model_validate_json(inputs),
        content=SubtitleContentV1.model_validate_json(content),
        content_hash=row["content_hash"],
        status=row["status"],
        resource_version=row["resource_version"],
        created_at=row["created_at"],
        updated_at=row["updated_at"],
        confirmed_at=row["confirmed_at"],
        input_outdated=bool(row["input_outdated"]),
    )


def with_projection(row: asyncpg.Record, **values: object) -> dict[str, object]:
    return {**dict(row), **values}


def _json_text(value: object) -> str:
    return value if isinstance(value, str) else json.dumps(value)


def _json_value(value: object) -> object:
    return json.loads(value) if isinstance(value, str) else value
