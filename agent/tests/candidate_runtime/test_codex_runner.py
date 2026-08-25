from __future__ import annotations

from typing import Any, cast

from app.modules.scripts.production_bibles.contracts import ProductionBibleCandidate
from app.modules.skills.harness import structured_diagnostic
from app.modules.storyboards.contracts import StoryboardCandidate


def _assert_strict_objects(value: Any) -> None:
    if isinstance(value, dict):
        mapping = cast(dict[str, Any], value)
        if mapping.get("type") == "object":
            properties = cast(dict[str, Any], mapping.get("properties", {}))
            required = cast(list[str], mapping.get("required", []))
            assert mapping.get("additionalProperties") is False
            assert set(required) == set(properties)
        for child in mapping.values():
            _assert_strict_objects(child)
    elif isinstance(value, list):
        for child in cast(list[Any], value):
            _assert_strict_objects(child)


def test_structured_diagnostic_prefers_codex_error_event() -> None:
    stdout = b'{"type":"thread.started","thread_id":"test"}\n'
    stdout += b'{"type":"error","message":"model request failed"}\n'

    result = structured_diagnostic(stdout, b"details {\n}\n")

    assert result == "model request failed"


def test_structured_diagnostic_falls_back_without_exposing_multiline_payload() -> None:
    result = structured_diagnostic(b"", b"request failed\ninternal diagnostic\n}\n")

    assert result == "internal diagnostic"


def test_candidate_output_schemas_are_strict_structured_outputs() -> None:
    _assert_strict_objects(ProductionBibleCandidate.model_json_schema())
    _assert_strict_objects(StoryboardCandidate.model_json_schema())
