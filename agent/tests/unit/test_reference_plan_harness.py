from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest

from app.modules.storygraph.harness import InvocationPolicyInvalid
from app.modules.storygraph.reference_plan_contract import (
    ReferencePlanCandidate,
    ReferencePlanInput,
)
from app.modules.storygraph.reference_plan_harness import ReferencePlanHarness
from app.reasoning.codex import CodexSchemaInvalid
from tests.contract.test_reference_plan_contract import valid_candidate, valid_input

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]


def test_reference_plan_harness_loads_only_its_declared_skill_resources() -> None:
    harness = ReferencePlanHarness(
        ReferencePlanInput.model_validate(valid_input()),
        repository_root=REPOSITORY_ROOT,
    )

    assert harness.bundle.loaded_paths("plan_reference_assets", "default") == (
        "SKILL.md",
        "references/reference-planning.md",
    )
    assert harness.bundle.verify_installed_bundle() == harness.bundle.manifest.skill_bundle_hash


def test_reference_plan_harness_rejects_budget_above_its_release() -> None:
    with pytest.raises(InvocationPolicyInvalid):
        ReferencePlanHarness(
            ReferencePlanInput.model_validate(valid_input()),
            max_execution_seconds=121,
            repository_root=REPOSITORY_ROOT,
        )


async def test_reference_plan_harness_runs_one_strict_candidate_call(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    stage_input = ReferencePlanInput.model_validate(valid_input())
    captured: dict[str, Any] = {}

    async def run(**kwargs: Any) -> ReferencePlanCandidate:
        captured.update(kwargs)
        return ReferencePlanCandidate.model_validate(valid_candidate(stage_input))

    monkeypatch.setattr(
        "app.modules.storygraph.reference_plan_harness.run_codex_process",
        run,
    )
    harness = ReferencePlanHarness(
        stage_input,
        max_execution_seconds=30,
        max_output_bytes=4096,
        repository_root=REPOSITORY_ROOT,
    )

    candidate = await harness.execute()

    candidate.validate_for(stage_input)
    assert harness.model_name == "codex-cli-default"
    assert captured["output_model"] is ReferencePlanCandidate
    assert captured["strict_output_schema"] is True
    assert captured["timeout_seconds"] == 30
    assert captured["max_output_bytes"] == 4096
    assert "references/reference-planning.md" in captured["guidance"]
    assert "references/visual-identity.md" not in captured["guidance"]


async def test_reference_plan_harness_rejects_candidate_lineage_drift(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    stage_input = ReferencePlanInput.model_validate(valid_input())
    drifted = ReferencePlanCandidate.model_validate(valid_candidate(stage_input)).model_copy(
        update={"visual_foundation_candidate_revision_hash": "f" * 64}
    )

    async def run(**_: Any) -> ReferencePlanCandidate:
        return drifted

    monkeypatch.setattr(
        "app.modules.storygraph.reference_plan_harness.run_codex_process",
        run,
    )

    with pytest.raises(CodexSchemaInvalid, match="changed frozen input"):
        await ReferencePlanHarness(stage_input, repository_root=REPOSITORY_ROOT).execute()
