import asyncio
from unittest.mock import AsyncMock

import pytest

from app.harness.service import HarnessService
from app.modules.text_storyboard.harness import (
    RELEASE_HASH,
    CandidateContractInvalid,
    ContextInsufficient,
    InputContractInvalid,
    SkillReleaseInvalid,
)
from app.protocol.canonical import canonical_hash
from app.reasoning.codex import CodexExecutionError
from app.skills.catalog import SkillCatalog
from app.skills.runtime import SkillRuntime
from app.text_contract.failure import HarnessFailed
from app.text_contract.task import TextTask
from tests.unit.text_storyboard_samples import sample


@pytest.mark.parametrize(
    ("error", "code", "state"),
    [
        (SkillReleaseInvalid(), "skill_release_unavailable", "failed"),
        (ContextInsufficient(), "context_insufficient", "failed"),
        (InputContractInvalid(), "input_contract_invalid", "failed"),
        (
            CandidateContractInvalid({"invalid": True}, "bad source"),
            "candidate_contract_invalid",
            "failed",
        ),
        (
            CodexExecutionError("unknown outcome"),
            "reasoning_execution_failed_or_unknown",
            "unknown",
        ),
    ],
)
async def test_local_invocation_preserves_task_bound_failure_receipt(
    monkeypatch: pytest.MonkeyPatch, error: Exception, code: str, state: str
) -> None:
    service = HarnessService(SkillRuntime(SkillCatalog()))
    task = TextTask(
        invocation_id="local", stage="map_manuscript", source=sample()[0], release_hash=RELEASE_HASH
    )
    monkeypatch.setattr(service, "text_storyboard", AsyncMock(side_effect=error))
    with pytest.raises(HarnessFailed) as caught:
        await service.invoke(task)
    failure = caught.value.failure
    assert (failure.code, failure.state) == (code, state)
    assert failure.invocation_id == task.invocation_id
    assert failure.input_hash == canonical_hash(task.model_dump(mode="json"))
    assert failure.release_hash == task.release_hash
    if isinstance(error, CandidateContractInvalid):
        assert failure.candidate == error.candidate


async def test_cancellation_propagates_to_execution_store_as_unknown(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    service = HarnessService(SkillRuntime(SkillCatalog()))
    task = TextTask(
        invocation_id="local", stage="map_manuscript", source=sample()[0], release_hash=RELEASE_HASH
    )
    monkeypatch.setattr(service, "text_storyboard", AsyncMock(side_effect=asyncio.CancelledError))
    with pytest.raises(asyncio.CancelledError):
        await service.invoke(task)
