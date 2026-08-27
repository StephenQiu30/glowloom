from __future__ import annotations

import json
from pathlib import Path
from typing import Any, cast

import pytest
from pydantic import ValidationError

from app.candidate_runtime.schemas import StoryGraphStageResult

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]


def test_error_codes_and_retry_semantics_match_the_backend_fixture() -> None:
    errors = cast(
        list[dict[str, Any]],
        json.loads(
            (REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-errors-v1.json").read_text(
                encoding="utf-8"
            )
        ),
    )
    assert len(errors) == 11
    base: dict[str, Any] = {
        "invocation_id": "20000000-0000-0000-0000-000000000001",
        "kind": "storygraph_stage",
        "wire_schema_version": "storygraph-stage-wire-v1",
        "stage": "extract_source_evidence",
        "shard_key": "slice-0001",
        "candidate_type": "source_evidence_candidate",
        "candidate": None,
        "input_hash": "a" * 64,
        "result_hash": None,
        "issues": [],
        "executor": {"name": "codex-cli", "version": "test", "model": "test"},
    }
    for error in errors:
        value = StoryGraphStageResult.model_validate(
            {
                **base,
                "status": error["status"],
                "error": {
                    "code": error["code"],
                    "summary": "test",
                    "retryable": error["retryable"],
                },
            }
        )
        assert value.error is not None
        with pytest.raises(ValidationError):
            StoryGraphStageResult.model_validate(
                {
                    **base,
                    "status": error["status"],
                    "error": {
                        "code": error["code"],
                        "summary": "test",
                        "retryable": not error["retryable"],
                    },
                }
            )
