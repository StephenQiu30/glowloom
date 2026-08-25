from __future__ import annotations

from typing import Any, cast
from uuid import uuid4

import pytest

from app.candidate_runtime import api
from app.candidate_runtime.schemas import Invocation
from app.modules.storyboards.contracts import StoryboardDraftInput


@pytest.mark.asyncio
async def test_storyboard_invocation_maps_backend_payload_to_candidate_input(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    unit_id = uuid4()
    scene_id = uuid4()
    captured: dict[str, Any] = {}

    class FakeDrafter:
        def __init__(self, **_: object) -> None:
            pass

        async def draft(self, value: object) -> dict[str, object]:
            captured["input"] = value
            return {
                "shots": [
                    {
                        "proposal_key": "shot-001",
                        "position": 1,
                        "title": "建立场景",
                        "narrative_unit_version_ids": [str(unit_id)],
                        "spec": {"duration_ms": 3000},
                        "asset_references": [],
                        "risk_codes": [],
                    }
                ]
            }

        async def aclose(self) -> None:
            captured["closed"] = True

    def allow_grant(_: str, __: str, ___: Invocation) -> None:
        return None

    monkeypatch.setattr(api, "CodexLocalStoryboardDrafter", FakeDrafter)
    monkeypatch.setattr(api, "verify_execution_grant", allow_grant)
    input_hash = "a" * 64
    invocation = Invocation(
        invocation_id=uuid4(),
        kind="storyboard_draft",
        input_hash=input_hash,
        schema_version="agent-candidate-v1",
        payload={
            "batch_id": str(uuid4()),
            "task_id": str(uuid4()),
            "input_hash": input_hash,
            "script_version_id": str(uuid4()),
            "target_duration_ms": 90_000,
            "aspect_ratio": "9:16",
            "visual_style": "水墨",
            "units": [
                {
                    "unit_version_id": str(unit_id),
                    "position": 1,
                    "kind": "scene_heading",
                    "exact_text": "内景·茶馆·日",
                    "required_for_coverage": True,
                    "source_scene_id": str(scene_id),
                    "source_dialogue_id": None,
                }
            ],
            "assets": [],
            "production_bible_id": str(uuid4()),
            "production_bible_revision": 2,
            "production_bible_result_hash": "b" * 64,
            "world_entries": [],
            "run_token": str(uuid4()),
        },
    )

    monkeypatch.setenv("AGENT_EXECUTION_SECRET", "a-secure-agent-execution-secret-value")
    result = await api.invoke(invocation, "test-grant")

    assert result.status == "succeeded"
    assert result.candidate is not None
    assert result.candidate["shots"][0]["proposal_key"] == "shot-001"  # type: ignore[index]
    captured_input = cast(StoryboardDraftInput, captured["input"])
    assert captured_input.units[0].source_scene_id == scene_id
    assert captured["closed"] is True
