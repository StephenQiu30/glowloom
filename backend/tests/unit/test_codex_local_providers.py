import json
from collections.abc import Sequence
from typing import Any, cast

import pytest
from langchain_core.messages import HumanMessage

from app.integrations.codex_local import CodexLocalEpisodePlanner, CodexLocalScriptAdapter


class RecordingModel:
    def __init__(self, result: object) -> None:
        self.result = result
        self.messages: Sequence[Any] = ()

    async def ainvoke(self, messages: Sequence[Any]) -> object:
        self.messages = messages
        return self.result


@pytest.mark.asyncio
async def test_codex_episode_planner_uses_numbered_fixed_source_blocks() -> None:
    model = RecordingModel(
        {
            "proposals": [
                {
                    "title": "第一集",
                    "end_block_position": 2,
                    "exact_end_anchor": "警报响起。",
                    "estimated_duration_ms": 60_000,
                    "reason": "危机钩子形成",
                    "confidence": 0.9,
                }
            ]
        }
    )
    planner = CodexLocalEpisodePlanner(model=model)

    result = await planner.plan(
        "沈岚进入泵站。\n警报响起。",
        target_duration_ms=60_000,
        maximum_episode_count=8,
    )

    payload = json.loads(cast(str, cast(HumanMessage, model.messages[1]).content))
    assert payload["source_blocks"] == [
        {"position": 1, "text": "沈岚进入泵站。"},
        {"position": 2, "text": "警报响起。"},
    ]
    assert result.proposals[0].end_block_position == 2


@pytest.mark.asyncio
async def test_codex_script_adapter_keeps_duration_and_plot_constraints_in_payload() -> None:
    model = RecordingModel(
        {
            "adapted_script_text": "内景·泵站·夜\n沈岚：快关闸！",
            "change_summary": "压缩动作并保留关闸冲突",
            "estimated_duration_ms": 60_000,
        }
    )
    adapter = CodexLocalScriptAdapter(model=model)

    result = await adapter.adapt(
        "沈岚进入泵站，发现水位上涨。",
        target_duration_ms=60_000,
        core_plot_points=["沈岚必须关闸"],
        pacing="fast",
        colloquial_dialogue=True,
    )

    payload = json.loads(cast(str, cast(HumanMessage, model.messages[1]).content))
    assert payload["duration_acceptance_range_ms"] == [45_000, 75_000]
    assert payload["core_plot_points"] == ["沈岚必须关闸"]
    assert payload["pacing"] == "fast"
    assert result.estimated_duration_ms == 60_000
