import json
import logging

from app.core.logging import JsonFormatter, sanitize


def test_sensitive_fields_and_control_characters_are_sanitized() -> None:
    result = sanitize(
        {
            "token": "secret-token",
            "nested": {"api_key": "secret-key"},
            "message": "first\nsecond",
        }
    )
    assert result == {
        "token": "[REDACTED]",
        "nested": {"api_key": "[REDACTED]"},
        "message": "first second",
    }


def test_json_formatter_emits_one_valid_line_without_secret() -> None:
    record = logging.LogRecord("test", logging.INFO, __file__, 1, "hello\nworld", (), None)
    record.context = {"password": "not-for-logs", "request_id": "request-1"}
    rendered = JsonFormatter().format(record)
    parsed = json.loads(rendered)
    assert "\n" not in rendered
    assert "not-for-logs" not in rendered
    assert parsed["password"] == "[REDACTED]"
