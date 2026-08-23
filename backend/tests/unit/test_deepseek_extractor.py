import json
from typing import Any

import pytest
from openai import LengthFinishReasonError
from openai.types.chat import ChatCompletion
from pydantic import SecretStr

from app.integrations import deepseek as deepseek_integration
from app.modules.scripts.extractions.schemas import ScriptExtractionResult
from app.modules.scripts.planning.schemas import EpisodePlanningProviderResult


def _structured_result() -> dict[str, Any]:
    return {
        "candidates": [
            {
                "candidate_key": "scene-001",
                "source_range": {"start": 0, "end": 3},
                "proposal": {
                    "kind": "scene",
                    "heading": "第一场",
                    "location": "室内",
                    "time_of_day": "白天",
                    "summary": "角色开始行动",
                },
                "confidence_note": "场次标题清晰",
            }
        ]
    }


@pytest.mark.asyncio
async def test_deepseek_extractor_uses_fixed_non_thinking_structured_contract(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    created: dict[str, Any] = {}

    class FakeStructuredModel:
        async def ainvoke(self, messages: list[object]) -> dict[str, Any]:
            created["messages"] = messages
            return _structured_result()

    class FakeChatDeepSeek:
        def __init__(self, **kwargs: Any) -> None:
            created["constructor"] = kwargs

        def with_structured_output(
            self,
            schema: type[ScriptExtractionResult],
            **kwargs: Any,
        ) -> FakeStructuredModel:
            created["schema"] = schema
            created["structured_output"] = kwargs
            return FakeStructuredModel()

    monkeypatch.setattr(
        deepseek_integration,
        "ChatDeepSeek",
        FakeChatDeepSeek,
    )
    extractor = deepseek_integration.DeepSeekScriptStructureExtractor(
        SecretStr("test-deepseek-key")
    )

    result = await extractor.extract("第一场\n角色甲：开始。")

    expected_result = _structured_result()
    expected_result["candidates"][0]["source_range"] = {
        "start": 0,
        "end": len("第一场\n角色甲：开始。"),
    }
    expected_result["candidates"].append(
        {
            "candidate_key": "tool-dialogue-5ac8a477a2d969c3",
            "source_range": {
                "start": len("第一场\n"),
                "end": len("第一场\n角色甲：开始。"),
            },
            "proposal": {
                "kind": "dialogue",
                "scene_candidate_key": "scene-001",
                "speaker_candidate": "角色甲",
                "dialogue_kind": "spoken",
                "text": "开始。",
            },
            "confidence_note": "由结构工具按原文补齐",
        }
    )
    assert result == ScriptExtractionResult.model_validate(expected_result)
    constructor = created["constructor"]
    assert constructor["model"] == "deepseek-v4-pro"
    assert constructor["base_url"] == "https://api.deepseek.com"
    assert constructor["temperature"] == 0
    assert constructor["max_retries"] == 0
    assert constructor["timeout"] == 120
    assert constructor["extra_body"] == {"thinking": {"type": "disabled"}}
    assert isinstance(constructor["api_key"], SecretStr)
    assert created["schema"] is ScriptExtractionResult
    assert created["structured_output"] == {"method": "json_mode"}

    messages = created["messages"]
    assert len(messages) == 2
    system_content = messages[0].content
    user_content = messages[1].content
    assert "JSON" in system_content
    assert '"candidates"' in system_content
    assert "只提取" in system_content
    assert json.loads(user_content)["script_text"] == "第一场\n角色甲：开始。"


def test_deepseek_extractor_snapshot_is_complete_and_bounded() -> None:
    snapshot = deepseek_integration.DEEPSEEK_SCRIPT_EXTRACTOR_VERSION

    assert snapshot == ("langgraph-map-reduce-v1:prompt-v5:schema-v3")
    assert len(snapshot) <= 80


def test_deepseek_extractor_maps_length_truncation_to_actionable_failure() -> None:
    completion = ChatCompletion.model_validate(
        {
            "id": "length-limited",
            "choices": [
                {
                    "finish_reason": "length",
                    "index": 0,
                    "message": {"role": "assistant", "content": ""},
                }
            ],
            "created": 0,
            "model": "deepseek-v4-pro",
            "object": "chat.completion",
            "usage": {
                "completion_tokens": 8192,
                "prompt_tokens": 1,
                "total_tokens": 8193,
            },
        }
    )

    error = deepseek_integration._provider_error(  # pyright: ignore[reportPrivateUsage]
        LengthFinishReasonError(completion=completion)
    )

    assert error.outcome == "failed"
    assert error.code == "ai_output_too_large"
    assert error.summary == "DeepSeek extraction output exceeded the response limit"
    assert error.retryable is False
    assert error.next_action == "start_new_extraction"


def test_deepseek_structure_prompt_keeps_storyboards_in_their_own_skill() -> None:
    prompt = deepseek_integration.script_structure_system_prompt()

    assert "禁止生成 shot 候选" in prompt
    assert "storyboard.plan" in prompt
    assert "Markdown 标题" in prompt
    assert "逐字复制原文中的场景标题行" in prompt


def test_deepseek_extractor_anchors_scene_and_dialogue_ranges_to_source() -> None:
    script = "内景·屋内·日\n林澈：开始。\n外景·路口·夜\n周岑：停下。"
    raw_result = ScriptExtractionResult.model_validate(
        {
            "candidates": [
                {
                    "candidate_key": "scene-1",
                    "source_range": {"start": 1, "end": len(script)},
                    "proposal": {
                        "kind": "scene",
                        "heading": "内景·屋内·日",
                        "location": "屋内",
                        "time_of_day": "日",
                        "summary": "林澈开始行动。",
                    },
                },
                {
                    "candidate_key": "dialogue-1",
                    "source_range": {"start": 1, "end": len(script)},
                    "proposal": {
                        "kind": "dialogue",
                        "scene_candidate_key": "scene-1",
                        "speaker_candidate": "林澈",
                        "dialogue_kind": "spoken",
                        "text": "开始。",
                    },
                },
                {
                    "candidate_key": "scene-2",
                    "source_range": {"start": 1, "end": len(script)},
                    "proposal": {
                        "kind": "scene",
                        "heading": "外景·路口·夜",
                        "location": "路口",
                        "time_of_day": "夜",
                        "summary": "周岑阻止行动。",
                    },
                },
                {
                    "candidate_key": "dialogue-2",
                    "source_range": {"start": 1, "end": len(script)},
                    "proposal": {
                        "kind": "dialogue",
                        "scene_candidate_key": "scene-2",
                        "speaker_candidate": "周岑",
                        "dialogue_kind": "spoken",
                        "text": "停下。",
                    },
                },
            ]
        }
    )

    result = deepseek_integration._anchor_script_structure_ranges(  # pyright: ignore[reportPrivateUsage]
        raw_result,
        script,
    )

    second_scene_start = script.index("外景·路口·夜")
    ranges = {
        item.candidate_key: (item.source_range.start, item.source_range.end)
        for item in result.candidates
    }
    assert ranges == {
        "scene-1": (0, second_scene_start),
        "dialogue-1": (
            script.index("林澈：开始。"),
            script.index("林澈：开始。") + len("林澈：开始。"),
        ),
        "scene-2": (second_scene_start, len(script)),
        "dialogue-2": (
            script.index("周岑：停下。"),
            script.index("周岑：停下。") + len("周岑：停下。"),
        ),
    }


@pytest.mark.asyncio
async def test_deepseek_episode_planner_numbers_source_blocks_in_structured_input(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    created: dict[str, Any] = {}
    provider_result = {
        "proposals": [
            {
                "title": "警报",
                "end_block_position": 3,
                "exact_end_anchor": "第三行。",
                "estimated_duration_ms": 45_000,
                "reason": "冲突建立",
                "confidence": 0.8,
            }
        ]
    }

    class FakeStructuredModel:
        async def ainvoke(self, messages: list[object]) -> dict[str, Any]:
            created["messages"] = messages
            return provider_result

    class FakeChatDeepSeek:
        def __init__(self, **kwargs: Any) -> None:
            created["constructor"] = kwargs

        def with_structured_output(
            self,
            schema: type[EpisodePlanningProviderResult],
            **kwargs: Any,
        ) -> FakeStructuredModel:
            created["schema"] = schema
            created["structured_output"] = kwargs
            return FakeStructuredModel()

    monkeypatch.setattr(deepseek_integration, "ChatDeepSeek", FakeChatDeepSeek)
    planner = deepseek_integration.DeepSeekEpisodePlanner(SecretStr("test-deepseek-key"))

    result = await planner.plan(
        "第一行。\n第二行。\n第三行。",
        target_duration_ms=60_000,
        maximum_episode_count=10,
    )

    assert result == EpisodePlanningProviderResult.model_validate(provider_result)
    assert created["schema"] is EpisodePlanningProviderResult
    assert created["structured_output"] == {"method": "json_mode"}
    messages = created["messages"]
    assert "最后一项必须等于 3" in messages[0].content
    assert "不是候选集序号" in messages[0].content
    assert messages[1].content == (
        '{"source_blocks":[{"position":1,"text":"第一行。"},'
        '{"position":2,"text":"第二行。"},{"position":3,"text":"第三行。"}]}'
    )
