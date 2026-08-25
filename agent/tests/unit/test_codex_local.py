import asyncio
import json
from hashlib import sha256
from pathlib import Path as SyncPath
from typing import Any, cast

import pytest
from anyio import Path
from langchain_core.messages import HumanMessage, SystemMessage

from app.integrations import codex_local
from app.modules.scripts.extractions.schemas import ScriptExtractionResult
from app.modules.scripts.planning.schemas import EpisodePlanningProviderResult
from app.modules.scripts.production_bibles.schemas import (
    BibleEvidenceChunkResult,
    ProductionBibleProviderResult,
)


@pytest.mark.asyncio
async def test_codex_exec_propagates_cancellation_after_stopping_process(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    class FakeProcess:
        def __init__(self) -> None:
            self.terminated = False
            self.killed = False

        async def communicate(self, _: bytes) -> tuple[bytes, bytes]:
            await asyncio.sleep(60)
            return b"", b""

        def terminate(self) -> None:
            self.terminated = True

        async def wait(self) -> int:
            return 0

        def kill(self) -> None:
            self.killed = True

    process = FakeProcess()

    async def fake_create_subprocess_exec(*_: str, **__: object) -> FakeProcess:
        return process

    monkeypatch.setattr(asyncio, "create_subprocess_exec", fake_create_subprocess_exec)

    task = asyncio.create_task(
        codex_local._run_codex_exec(  # pyright: ignore[reportPrivateUsage]
            ["/local/codex", "exec"], "{}"
        )
    )
    await asyncio.sleep(0)
    task.cancel()

    with pytest.raises(asyncio.CancelledError):
        await task

    assert process.terminated is True
    assert process.killed is False


@pytest.mark.asyncio
async def test_codex_exec_reports_stdout_when_the_cli_fails(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    class FailedProcess:
        returncode = 1

        async def communicate(self, _: bytes) -> tuple[bytes, bytes]:
            return b"output schema is invalid\n", b""

    async def fake_create_subprocess_exec(*_: str, **__: object) -> FailedProcess:
        return FailedProcess()

    monkeypatch.setattr(asyncio, "create_subprocess_exec", fake_create_subprocess_exec)

    with pytest.raises(RuntimeError, match="output schema is invalid"):
        await codex_local._run_codex_exec(  # pyright: ignore[reportPrivateUsage]
            ["/local/codex", "exec"], "{}"
        )


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
                    "production_tasks": [
                        {
                            "task_type": "shot_breakdown",
                            "title": "拆解第一场分镜",
                            "objective": "将第一场拆解为可审核镜头。",
                            "priority": "normal",
                        }
                    ],
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
    ) -> None:
        calls["command"] = command
        calls["prompt"] = prompt
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
    assert "--config" not in command
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
    ) -> None:
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
        skill_name="review-shots",
    )

    result = await model.ainvoke([SystemMessage(content="规划分集"), HumanMessage(content="{}")])

    assert isinstance(result, EpisodePlanningProviderResult)
    assert calls["command"][0] == "/system/codex"
    assert "--model" not in calls["command"]
    assert "--config" not in calls["command"]
    assert calls["prompt"].startswith("$review-shots\n\n")
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
    ) -> None:
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


@pytest.mark.asyncio
async def test_codex_local_derives_bible_evidence_hash_before_validation(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    anchor = "Aurelia"

    async def fake_exec(command: list[str], _: str) -> None:
        response_path = Path(command[command.index("--output-last-message") + 1])
        await response_path.write_text(
            json.dumps(
                {
                    "chunk_key": "chunk-0001",
                    "source_start": 0,
                    "source_end": len(anchor),
                    "observations": [
                        {
                            "observation_key": "aurelia-mention",
                            "kind": "entity",
                            "subject_key": "aurelia",
                            "parent_entity_key": None,
                            "claim": "Aurelia is named.",
                            "evidence": [
                                {
                                    "source_start": 0,
                                    "source_end": len(anchor),
                                    "text_hash": "0" * 64,
                                    "exact_anchor": anchor,
                                    "episode_number": 1,
                                }
                            ],
                            "ambiguities": [],
                        }
                    ],
                }
            ),
            encoding="utf-8",
        )

    monkeypatch.setattr(codex_local, "_run_codex_exec", fake_exec)
    model = codex_local.CodexLocalStructuredModel(
        output_model=BibleEvidenceChunkResult,
        service_name="lanverse-extract-bible-evidence",
        codex_cli_path="/local/codex",
    )

    result = await model.ainvoke([HumanMessage(content="{}")])

    assert result.observations[0].evidence[0].text_hash == sha256(anchor.encode()).hexdigest()


@pytest.mark.asyncio
async def test_codex_local_derives_bible_normalized_name_before_validation(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    anchor = "Lady Míra"

    async def fake_exec(command: list[str], _: str) -> None:
        response_path = Path(command[command.index("--output-last-message") + 1])
        await response_path.write_text(
            json.dumps(
                {
                    "entities": [
                        {
                            "entity_key": "character.lady_mira",
                            "kind": "character",
                            "canonical_name": "  Lady   Míra  ",
                            "normalized_name": "WRONG",
                            "aliases": [],
                            "stable_spec": {"identity": "The named noblewoman."},
                            "episode_numbers": [1],
                            "evidence": [
                                {
                                    "source_start": 0,
                                    "source_end": len(anchor),
                                    "text_hash": "0" * 64,
                                    "exact_anchor": anchor,
                                    "episode_number": 1,
                                }
                            ],
                            "states": [],
                            "ambiguities": [],
                        }
                    ],
                    "world_entries": [],
                    "review_issues": [],
                },
                ensure_ascii=False,
            ),
            encoding="utf-8",
        )

    monkeypatch.setattr(codex_local, "_run_codex_exec", fake_exec)
    model = codex_local.CodexLocalStructuredModel(
        output_model=ProductionBibleProviderResult,
        service_name="lanverse-reconcile-bible",
        codex_cli_path="/local/codex",
    )

    result = await model.ainvoke([HumanMessage(content="{}")])

    assert result.entities[0].normalized_name == "lady míra"


def test_codex_production_bible_schema_has_no_open_objects() -> None:
    schema = codex_local._codex_output_schema(  # pyright: ignore[reportPrivateUsage]
        ProductionBibleProviderResult
    )
    open_paths: list[str] = []

    def inspect(value: object, path: str) -> None:
        if isinstance(value, dict):
            mapping = cast(dict[str, object], value)
            if mapping.get("type") == "object" and mapping.get("additionalProperties") is not False:
                open_paths.append(path)
            for key, child in mapping.items():
                inspect(child, f"{path}.{key}")
        elif isinstance(value, list):
            for index, child in enumerate(cast(list[object], value)):
                inspect(child, f"{path}[{index}]")

    inspect(schema, "schema")

    assert open_paths == []


def _write_project_storyboard_skill_pack(
    tmp_path: SyncPath,
) -> None:
    for skill_name in codex_local.STORYBOARD_SKILL_NAMES:
        skill_dir = tmp_path / ".agents" / "skills" / skill_name
        policy_dir = skill_dir / "agents"
        policy_dir.mkdir(parents=True)
        (skill_dir / "SKILL.md").write_text(
            f"---\nname: {skill_name}\ndescription: Test skill.\n---\n",
            encoding="utf-8",
        )
        (policy_dir / "openai.yaml").write_text(
            "interface:\n"
            f'  default_prompt: "Use ${skill_name} for this stage."\n'
            "policy:\n"
            "  allow_implicit_invocation: false\n",
            encoding="utf-8",
        )
    for relative_path in codex_local.STORYBOARD_SKILL_REFERENCE_PATHS:
        reference_path = tmp_path / relative_path
        reference_path.parent.mkdir(parents=True, exist_ok=True)
        reference_path.write_text("# Project-owned reference\n", encoding="utf-8")


def test_storyboard_skill_pack_is_project_owned_and_uses_generic_names(
    tmp_path: SyncPath,
) -> None:
    _write_project_storyboard_skill_pack(tmp_path)

    codex_local.verify_storyboard_skills(tmp_path)

    assert codex_local.STORYBOARD_SKILL_NAMES == (
        "analyze-scene",
        "plan-scene",
        "draft-shots",
        "review-shots",
        "repair-shots",
    )
    assert all(not name.startswith("lanverse-") for name in codex_local.STORYBOARD_SKILL_NAMES)
    assert all(
        relative_path.parts[:2] == (".agents", "skills")
        for relative_path in codex_local.STORYBOARD_SKILL_REFERENCE_PATHS
    )


def test_storyboard_skill_pack_fails_closed_when_project_reference_is_missing(
    tmp_path: SyncPath,
) -> None:
    _write_project_storyboard_skill_pack(tmp_path)
    missing_relative_path = codex_local.STORYBOARD_SKILL_REFERENCE_PATHS[0]
    (tmp_path / missing_relative_path).unlink()

    with pytest.raises(RuntimeError, match=str(missing_relative_path)):
        codex_local.verify_storyboard_skills(tmp_path)


def test_storyboard_skill_pack_fails_closed_when_default_prompt_name_drifts(
    tmp_path: SyncPath,
) -> None:
    _write_project_storyboard_skill_pack(tmp_path)
    skill_name = codex_local.STORYBOARD_SKILL_NAMES[0]
    policy_path = tmp_path / ".agents" / "skills" / skill_name / "agents" / "openai.yaml"
    policy_path.write_text(
        'interface:\n  default_prompt: "Use $wrong-name for this stage."\n'
        "policy:\n  allow_implicit_invocation: false\n",
        encoding="utf-8",
    )

    with pytest.raises(RuntimeError, match="default prompt does not match"):
        codex_local.verify_storyboard_skills(tmp_path)


def _write_project_production_bible_skill_pack(tmp_path: SyncPath) -> None:
    for skill_name in codex_local.PRODUCTION_BIBLE_SKILL_NAMES:
        skill_dir = tmp_path / ".agents" / "skills" / skill_name
        policy_dir = skill_dir / "agents"
        policy_dir.mkdir(parents=True)
        (skill_dir / "SKILL.md").write_text(
            f"---\nname: {skill_name}\ndescription: Test skill.\n---\n",
            encoding="utf-8",
        )
        (policy_dir / "openai.yaml").write_text(
            "interface:\n"
            f'  default_prompt: "Use ${skill_name} for this stage."\n'
            "policy:\n"
            "  allow_implicit_invocation: false\n",
            encoding="utf-8",
        )


def test_production_bible_skill_pack_uses_generic_action_names(
    tmp_path: SyncPath,
) -> None:
    _write_project_production_bible_skill_pack(tmp_path)

    codex_local.verify_production_bible_skills(tmp_path)

    assert codex_local.PRODUCTION_BIBLE_SKILL_NAMES == (
        "extract-bible-evidence",
        "reconcile-bible",
        "review-bible",
    )
    assert all(
        not name.startswith("lanverse-") for name in codex_local.PRODUCTION_BIBLE_SKILL_NAMES
    )
