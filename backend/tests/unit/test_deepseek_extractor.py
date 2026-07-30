from typing import Any

import pytest
from pydantic import SecretStr

from app.integrations import deepseek as deepseek_integration
from app.modules.scripts.extractions.schemas import ScriptExtractionResult


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

    assert result == ScriptExtractionResult.model_validate(_structured_result())
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
    assert user_content == "第一场\n角色甲：开始。"


def test_deepseek_extractor_snapshot_is_complete_and_bounded() -> None:
    snapshot = deepseek_integration.DEEPSEEK_SCRIPT_EXTRACTOR_VERSION

    assert snapshot == (
        "deepseek-v4-pro:thinking-off:lc-deepseek-1.1.0:prompt-v1:schema-v1"
    )
    assert len(snapshot) <= 80
