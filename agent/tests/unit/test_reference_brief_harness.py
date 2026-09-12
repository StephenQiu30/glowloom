from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest

from app.modules.storygraph.harness import InvocationPolicyInvalid
from app.modules.storygraph.reference_brief_contract import (
    ReferenceBriefCandidate,
    ReferenceBriefInput,
)
from app.modules.storygraph.reference_brief_harness import ReferenceBriefHarness
from app.reasoning.codex import CodexSchemaInvalid
from tests.contract.test_reference_brief_contract import (
    reference_brief_candidate,
    reference_brief_input,
)

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]


def stage_input() -> ReferenceBriefInput:
    candidate = reference_brief_candidate("character_appearance")
    return ReferenceBriefInput.model_validate(reference_brief_input(candidate))


def test_reference_brief_harness_loads_only_its_declared_skill_resources() -> None:
    harness = ReferenceBriefHarness(stage_input(), repository_root=REPOSITORY_ROOT)

    assert harness.bundle.loaded_paths("compile_reference_brief", "default") == (
        "SKILL.md",
        "references/reference-brief.md",
    )
    assert harness.bundle.verify_installed_bundle() == harness.bundle.manifest.skill_bundle_hash


def test_reference_brief_harness_rejects_budget_above_its_release() -> None:
    with pytest.raises(InvocationPolicyInvalid):
        ReferenceBriefHarness(
            stage_input(),
            max_execution_seconds=121,
            repository_root=REPOSITORY_ROOT,
        )


async def test_reference_brief_harness_runs_one_strict_candidate_call(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    frozen = stage_input()
    captured: dict[str, Any] = {}

    async def run(**kwargs: Any) -> ReferenceBriefCandidate:
        captured.update(kwargs)
        return ReferenceBriefCandidate.model_validate(
            reference_brief_candidate("character_appearance")
        )

    monkeypatch.setattr(
        "app.modules.storygraph.reference_brief_harness.run_codex_process",
        run,
    )
    harness = ReferenceBriefHarness(
        frozen,
        max_execution_seconds=30,
        max_output_bytes=4096,
        repository_root=REPOSITORY_ROOT,
    )

    candidate = await harness.execute()

    candidate.validate_for(frozen)
    assert harness.model_name == "codex-cli-default"
    assert captured["output_model"] is ReferenceBriefCandidate
    assert captured["strict_output_schema"] is True
    assert captured["timeout_seconds"] == 30
    assert captured["max_output_bytes"] == 4096
    assert "references/reference-brief.md" in captured["guidance"]
    assert "references/reference-planning.md" not in captured["guidance"]


async def test_reference_brief_harness_rejects_candidate_read_set_drift(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    frozen = stage_input()
    drifted = ReferenceBriefCandidate.model_validate(
        reference_brief_candidate("character_appearance")
    ).model_copy(update={"typed_read_set_root": "f" * 64})

    async def run(**_: Any) -> ReferenceBriefCandidate:
        return drifted

    monkeypatch.setattr(
        "app.modules.storygraph.reference_brief_harness.run_codex_process",
        run,
    )

    with pytest.raises(CodexSchemaInvalid, match="changed frozen input"):
        await ReferenceBriefHarness(frozen, repository_root=REPOSITORY_ROOT).execute()
