from __future__ import annotations

from app.modules.skills.harness import structured_diagnostic


def test_structured_diagnostic_prefers_codex_error_event() -> None:
    stdout = b'{"type":"thread.started","thread_id":"test"}\n'
    stdout += b'{"type":"error","message":"model request failed"}\n'

    result = structured_diagnostic(stdout, b"details {\n}\n")

    assert result == "model request failed"


def test_structured_diagnostic_falls_back_without_exposing_multiline_payload() -> None:
    result = structured_diagnostic(b"", b"request failed\ninternal diagnostic\n}\n")

    assert result == "internal diagnostic"
