from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from app.candidate_runtime.schemas import ExecutionPolicy
from app.modules.skills.harness import CodexSchemaRunner
from app.modules.storyboards.contracts import StoryboardCandidate, StoryboardDraftInput


class CodexLocalStoryboardDrafter:
    def __init__(
        self,
        *,
        execution_policy: ExecutionPolicy,
        repository_root: Path | None = None,
    ) -> None:
        self._runner = CodexSchemaRunner(
            repository_root=repository_root, execution_policy=execution_policy
        )

    @property
    def model_name(self) -> str:
        return self._runner.model_name

    async def draft(self, value: StoryboardDraftInput) -> dict[str, Any]:
        candidate = await self._runner.run(
            "Use $draft-shots and read its shot-table rules. Return only schema-valid JSON. "
            "Create a review-only storyboard candidate from the immutable units. Preserve source "
            "order, use only supplied unit UUIDs, cover every required unit at least once, keep "
            "positions contiguous from 1, and set spec.duration_ms to an integer from 500 through "
            "15000. Do not persist, approve, or apply anything. Input:\n"
            + json.dumps(value.model_dump(mode="json"), ensure_ascii=False, separators=(",", ":")),
            StoryboardCandidate,
            skill_name="draft-shots",
        )
        return _normalize(candidate, value)

    async def aclose(self) -> None:
        await self._runner.aclose()


def _normalize(candidate: StoryboardCandidate, source: StoryboardDraftInput) -> dict[str, Any]:
    known = {str(unit.unit_version_id) for unit in source.units}
    required = {str(unit.unit_version_id) for unit in source.units if unit.required_for_coverage}
    shots: list[dict[str, Any]] = []
    keys: set[str] = set()
    for shot in sorted(
        candidate.model_dump(mode="json")["shots"], key=lambda item: item["position"]
    ):
        references = list(
            dict.fromkeys(
                unit_id for unit_id in shot["narrative_unit_version_ids"] if unit_id in known
            )
        )
        duration = shot["spec"].get("duration_ms")
        if not references or not isinstance(duration, int) or not 500 <= duration <= 15000:
            continue
        key = shot["proposal_key"]
        if not key or key in keys:
            key = f"shot-{len(shots) + 1:03d}"
        keys.add(key)
        shot["proposal_key"] = key
        shot["position"] = len(shots) + 1
        shot["narrative_unit_version_ids"] = references
        shots.append(shot)
    if not shots:
        raise ValueError("storyboard candidate contains no valid shots")
    covered = {unit_id for shot in shots for unit_id in shot["narrative_unit_version_ids"]}
    missing = required - covered
    if missing:
        raise ValueError("storyboard candidate does not cover every required source unit")
    return {"shots": shots}
