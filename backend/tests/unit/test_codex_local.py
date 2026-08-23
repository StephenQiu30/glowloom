import json
from typing import Any

import pytest
from anyio import Path
from langchain_core.messages import HumanMessage, SystemMessage

from app.integrations import codex_local
from app.modules.scripts.extractions.schemas import ScriptExtractionResult
from app.modules.scripts.planning.schemas import EpisodePlanningProviderResult


def _extraction_result() -> dict[str, Any]:
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
async def test_codex_local_model_uses_installed_read_only_ephemeral_json_contract(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: dict[str, Any] = {}

    async def fake_exec(
        command: list[str],
        prompt: str,
        *,
        timeout_seconds: float,
    ) -> None:
        calls["command"] = command
        calls["prompt"] = prompt
        calls["timeout_seconds"] = timeout_seconds
        schema_path = Path(command[command.index("--output-schema") + 1])
        calls["schema"] = json.loads(await schema_path.read_text(encoding="utf-8"))
        response_path = Path(command[command.index("--output-last-message") + 1])
        await response_path.write_text(
            json.dumps(_extraction_result(), ensure_ascii=False),
            encoding="utf-8",
        )

    monkeypatch.setattr(codex_local, "_run_codex_exec", fake_exec)
    model = codex_local.CodexLocalStructuredModel(
        output_model=ScriptExtractionResult,
        service_name="lanverse-script-structure",
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

    assert result == ScriptExtractionResult.model_validate(_extraction_result())
    command = calls["command"]
    assert command[:6] == [
        "/local/codex",
        "exec",
        "--ephemeral",
        "--sandbox",
        "read-only",
        "--cd",
    ]
    assert str(codex_local.REPOSITORY_ROOT) in command
    assert 'model_reasoning_effort="low"' in command
    assert command[-3:] == ["--model", "local-model", "-"]
    assert calls["schema"]["title"] == "ScriptExtractionResult"
    assert "aliases" in calls["schema"]["$defs"]["AssetCandidateProposal"]["required"]
    assert "只返回 JSON" in calls["prompt"]
    assert '{"script_text":"第一场"}' in calls["prompt"]


@pytest.mark.asyncio
async def test_codex_local_model_uses_configured_schema_and_explicit_skill(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: dict[str, Any] = {}

    def system_codex(_: str) -> str:
        return "/system/codex"

    monkeypatch.setattr(codex_local.shutil, "which", system_codex)

    async def fake_exec(
        command: list[str],
        prompt: str,
        *,
        timeout_seconds: float,
    ) -> None:
        del timeout_seconds
        calls["command"] = command
        calls["prompt"] = prompt
        schema_path = Path(command[command.index("--output-schema") + 1])
        calls["schema"] = json.loads(await schema_path.read_text(encoding="utf-8"))
        response_path = Path(command[command.index("--output-last-message") + 1])
        await response_path.write_text(
            json.dumps(
                {
                    "proposals": [
                        {
                            "title": "第一集",
                            "end_block_position": 1,
                            "exact_end_anchor": "警报响起。",
                            "estimated_duration_ms": 60_000,
                            "reason": "在危机钩子处收束",
                            "confidence": 0.9,
                        }
                    ]
                },
                ensure_ascii=False,
            ),
            encoding="utf-8",
        )

    monkeypatch.setattr(codex_local, "_run_codex_exec", fake_exec)
    model = codex_local.CodexLocalStructuredModel(
        output_model=EpisodePlanningProviderResult,
        service_name="lanverse-episode-planning",
        skill_name="storyboard-review",
    )

    result = await model.ainvoke([SystemMessage(content="规划分集"), HumanMessage(content="{}")])

    assert isinstance(result, EpisodePlanningProviderResult)
    assert calls["command"][0] == "/system/codex"
    assert calls["prompt"].startswith("$storyboard-review\n\n")
    assert calls["schema"]["title"] == "EpisodePlanningProviderResult"
    assert "工作流标识：lanverse-episode-planning" in calls["prompt"]


@pytest.mark.asyncio
async def test_codex_local_model_retries_one_invalid_structured_candidate(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    prompts: list[str] = []

    async def fake_exec(
        command: list[str],
        prompt: str,
        *,
        timeout_seconds: float,
    ) -> None:
        del timeout_seconds
        prompts.append(prompt)
        response_path = Path(command[command.index("--output-last-message") + 1])
        valid = _extraction_result()
        payload: dict[str, object] = (
            {"candidates": [*valid["candidates"], *valid["candidates"]]}
            if len(prompts) == 1
            else valid
        )
        await response_path.write_text(
            json.dumps(payload, ensure_ascii=False),
            encoding="utf-8",
        )

    monkeypatch.setattr(codex_local, "_run_codex_exec", fake_exec)
    model = codex_local.CodexLocalStructuredModel(
        output_model=ScriptExtractionResult,
        service_name="lanverse-script-structure",
        codex_cli_path="/local/codex",
        validation_attempts=2,
    )

    result = await model.ainvoke([HumanMessage(content="{}")])

    assert result.candidates[0].candidate_key == "scene-001"
    assert len(prompts) == 2
    assert "上一次候选未通过结构校验" in prompts[1]


def test_storyboard_skill_pack_uses_generic_names_and_pinned_upstream() -> None:
    codex_local.verify_storyboard_skills()

    assert codex_local.STORYBOARD_SKILL_NAMES == (
        "storyboard-source-analysis",
        "storyboard-scene-plan",
        "storyboard-shot-draft",
        "storyboard-review",
        "storyboard-repair",
    )
    assert all(not name.startswith("lanverse-") for name in codex_local.STORYBOARD_SKILL_NAMES)
