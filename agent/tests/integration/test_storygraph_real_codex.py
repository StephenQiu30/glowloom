from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
from typing import Any, cast

import pytest

from app.candidate_runtime.schemas import SourceEvidenceStageInput, StoryGraphStageInvocation
from app.modules.storygraph.candidate_schemas import SourceEvidenceCandidate
from app.modules.storygraph.harness import StoryGraphHarness

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
WIRE_FIXTURE = REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-stage-wire-v1.json"


@pytest.mark.skipif(
    os.getenv("LANVERSE_TEST_REAL_CODEX") != "1",
    reason="set LANVERSE_TEST_REAL_CODEX=1 to exercise the locally authenticated Codex CLI",
)
@pytest.mark.asyncio
async def test_real_codex_extracts_auditable_source_evidence() -> None:
    fixture = cast(dict[str, Any], json.loads(WIRE_FIXTURE.read_text(encoding="utf-8")))
    invocation = StoryGraphStageInvocation.model_validate(fixture["valid_invocation"])
    source_input = SourceEvidenceStageInput.model_validate(invocation.payload.stage_input)

    harness = StoryGraphHarness(invocation, repository_root=REPOSITORY_ROOT)
    result = await harness.execute()

    assert isinstance(result, SourceEvidenceCandidate)
    assert result.observations
    for observation in result.observations:
        for evidence in observation.evidence:
            local_start = evidence.source_start - source_input.context_start
            local_end = evidence.source_end - source_input.context_start
            assert source_input.normalized_text[local_start:local_end] == evidence.exact_anchor
            assert (
                hashlib.sha256(evidence.exact_anchor.encode("utf-8")).hexdigest()
                == evidence.text_hash
            )
