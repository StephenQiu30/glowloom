import json
from typing import Any

import pytest
from langchain_core.messages import HumanMessage, SystemMessage

from app.integrations import codex_local
from app.modules.scripts.extractions.schemas import ScriptExtractionResult


def _result() -> dict[str, Any]:
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
            }
        ]
    }


@pytest.mark.asyncio
async def test_codex_local_model_uses_read_only_ephemeral_json_contract(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: dict[str, Any] = {}

    class FakeTurn:
        final_response = json.dumps(_result(), ensure_ascii=False)

    class FakeThread:
        async def run(self, prompt: str, **kwargs: Any) -> FakeTurn:
            calls["prompt"] = prompt
            calls["turn"] = kwargs
            return FakeTurn()

    class FakeCodex:
        def __init__(self, config: Any) -> None:
            calls["config"] = config

        async def __aenter__(self) -> "FakeCodex":
            calls["initialized"] = True
            return self

        async def close(self) -> None:
            calls["closed"] = True

        async def thread_start(self, **kwargs: Any) -> FakeThread:
            calls["thread"] = kwargs
            return FakeThread()

    monkeypatch.setattr(codex_local, "AsyncCodex", FakeCodex)
    model = codex_local.CodexLocalStructuredModel(
        codex_cli_path="/local/codex",
        model="local-model",
        max_concurrency=1,
    )

    result = await model.ainvoke(
        [
            SystemMessage(content="只返回 JSON"),
            HumanMessage(content='{"script_text":"第一场"}'),
        ]
    )
    await model.aclose()

    assert result == ScriptExtractionResult.model_validate(_result())
    assert calls["initialized"] is True
    assert calls["thread"]["approval_mode"] == codex_local.ApprovalMode.deny_all
    assert calls["thread"]["sandbox"] == codex_local.Sandbox.read_only
    assert calls["thread"]["ephemeral"] is True
    assert calls["turn"]["sandbox"] == codex_local.Sandbox.read_only
    output_schema = calls["turn"]["output_schema"]
    assert output_schema["title"] == "ScriptExtractionResult"
    assert "aliases" in output_schema["$defs"]["AssetCandidateProposal"]["required"]
    assert calls["prompt"] == '{"script_text":"第一场"}'
    assert calls["closed"] is True
