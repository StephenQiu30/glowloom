import json
import logging
from typing import Protocol, cast

import pytest
from prometheus_client import REGISTRY

from app.core import logging as logging_module
from app.core.logging import (
    MAX_LOG_STRING_LENGTH,
    JsonFormatter,
    log_event,
    sanitize,
)


class _EventLogRecord(Protocol):
    event_name: str
    context: dict[str, object]


def test_sensitive_fields_and_control_characters_are_sanitized() -> None:
    result = sanitize(
        {
            "token": "secret-token",
            "nested": {"api_key": "secret-key"},
            "message": "first\nsecond",
            "presigned_url": "https://storage.example/secret",
            "long_value": "x" * (MAX_LOG_STRING_LENGTH + 10),
        }
    )
    assert result == {
        "token": "[REDACTED]",
        "nested": {"api_key": "[REDACTED]"},
        "message": "first second",
        "presigned_url": "[REDACTED]",
        "long_value": "x" * MAX_LOG_STRING_LENGTH,
    }


def test_json_formatter_emits_one_valid_line_without_secret() -> None:
    record = logging.LogRecord("test", logging.INFO, __file__, 1, "hello\nworld", (), None)
    record.context = {"password": "not-for-logs", "request_id": "request-1"}
    rendered = JsonFormatter().format(record)
    parsed = json.loads(rendered)
    assert "\n" not in rendered
    assert "not-for-logs" not in rendered
    assert parsed["password"] == "[REDACTED]"
    assert parsed["service"] == "lanverse"
    assert parsed["environment"] == "development"


def test_registered_log_event_drops_unknown_fields_and_counts_them(
    caplog: pytest.LogCaptureFixture,
) -> None:
    labels = {"signal": "log", "reason": "attribute_not_allowed"}
    before = (
        REGISTRY.get_sample_value(
            "lanverse_telemetry_redaction_drops_total",
            labels,
        )
        or 0
    )

    caplog.set_level(logging.INFO, logger="lanverse.http")
    log_event(
        logging.getLogger("lanverse.http"),
        logging.INFO,
        "http.request.completed",
        "request completed",
        request_id="019fcb55-b4bc-70e5-a123-123456789abc",
        method="GET",
        route="/api/v1/tasks/{task_id}",
        status_class="2xx",
        duration_ms=1.25,
        authorization="Bearer must-not-be-logged",
        arbitrary_payload="must-not-be-logged",
    )

    record = cast(_EventLogRecord, caplog.records[-1])
    assert record.event_name == "http.request.completed"
    assert record.context == {
        "request_id": "019fcb55-b4bc-70e5-a123-123456789abc",
        "method": "GET",
        "route": "/api/v1/tasks/{task_id}",
        "status_class": "2xx",
        "duration_ms": 1.25,
    }
    assert "must-not-be-logged" not in str(vars(caplog.records[-1]))
    after = REGISTRY.get_sample_value(
        "lanverse_telemetry_redaction_drops_total",
        labels,
    )
    assert after == before + 2


def test_storage_and_probe_events_only_keep_registered_attributes(
    caplog: pytest.LogCaptureFixture,
) -> None:
    object_key = "private/workspace/media-version.bin"
    endpoint = "127.0.0.1:9000"
    caplog.set_level(logging.INFO)

    log_event(
        logging.getLogger("lanverse.storage"),
        logging.WARNING,
        "storage.operation.failed",
        "object storage operation failed",
        trace_id="1" * 32,
        span_id="2" * 16,
        storage_profile="default",
        operation="stat",
        result="not_found",
        duration_ms=1.25,
        error_code="not_found",
        object_key=object_key,
        endpoint=endpoint,
    )
    storage_record = cast(_EventLogRecord, caplog.records[-1])
    assert storage_record.context == {
        "trace_id": "1" * 32,
        "span_id": "2" * 16,
        "storage_profile": "default",
        "operation": "stat",
        "result": "not_found",
        "duration_ms": 1.25,
        "error_code": "not_found",
    }

    log_event(
        logging.getLogger("lanverse.media.probe"),
        logging.INFO,
        "media.probe.completed",
        "media probe completed",
        trace_id="3" * 32,
        span_id="4" * 16,
        kind="image",
        result="succeeded",
        duration_ms=2.5,
        temporary_path="/tmp/private-input.png",
        stderr="private diagnostic",
    )
    probe_record = cast(_EventLogRecord, caplog.records[-1])
    assert probe_record.context == {
        "trace_id": "3" * 32,
        "span_id": "4" * 16,
        "kind": "image",
        "result": "succeeded",
        "duration_ms": 2.5,
    }
    rendered_records = str([vars(record) for record in caplog.records])
    assert object_key not in rendered_records
    assert endpoint not in rendered_records
    assert "/tmp/private-input.png" not in rendered_records
    assert "private diagnostic" not in rendered_records


def test_process_logging_configuration_is_first_writer_wins(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    calls: list[dict[str, object]] = []

    def record_configuration(**kwargs: object) -> None:
        calls.append(kwargs)

    monkeypatch.setattr(logging_module, "_logging_configured", False)
    monkeypatch.setattr(logging, "basicConfig", record_configuration)

    logging_module.configure_logging(
        "INFO",
        service="lanverse-server",
        environment="test",
    )
    logging_module.configure_logging(
        "DEBUG",
        service="lanverse-worker",
        environment="test",
    )

    assert len(calls) == 1
    handlers = cast(list[logging.Handler], calls[0]["handlers"])
    handler = handlers[0]
    formatter = cast(JsonFormatter, handler.formatter)
    rendered = json.loads(
        formatter.format(
            logging.LogRecord("test", logging.INFO, __file__, 1, "configured", (), None)
        )
    )
    assert rendered["service"] == "lanverse-server"
