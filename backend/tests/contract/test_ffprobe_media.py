import logging
import os
from collections.abc import AsyncIterator
from typing import Protocol, cast

import pytest
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from prometheus_client import REGISTRY

from app.core.telemetry import configure_telemetry
from app.integrations.ffprobe import FfprobeMediaProbe
from app.modules.media import MediaProbeError
from tests.support.media_fixtures import ONE_PIXEL_PNG


class _EventLogRecord(Protocol):
    event_name: str
    context: dict[str, object]


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_FFPROBE_CONTRACT") != "1",
    reason="set LANVERSE_RUN_FFPROBE_CONTRACT=1 with ffprobe 8.1.2 installed",
)
@pytest.mark.asyncio
async def test_ffprobe_inspects_real_image_bytes(
    caplog: pytest.LogCaptureFixture,
) -> None:
    async def content() -> AsyncIterator[bytes]:
        yield ONE_PIXEL_PNG

    provider = configure_telemetry(
        service_name="lanverse-ffprobe-contract",
        environment="test",
    )
    exporter = InMemorySpanExporter()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    exporter.clear()
    labels = {"kind": "image", "result": "succeeded"}
    before = REGISTRY.get_sample_value("lanverse_media_probe_results_total", labels) or 0
    caplog.set_level(logging.INFO, logger="lanverse.media.probe")

    result = await FfprobeMediaProbe(timeout_seconds=10).probe(
        content(), kind="image", mime_type="image/png"
    )

    assert result.width == 1
    assert result.height == 1
    assert result.codec == "png"
    assert result.container == "png_pipe"
    assert REGISTRY.get_sample_value("lanverse_media_probe_results_total", labels) == (
        before + 1
    )
    spans = [span for span in exporter.get_finished_spans() if span.name == "media.ffprobe"]
    assert len(spans) == 1
    assert spans[0].attributes == {
        "media.tool": "ffprobe",
        "media.kind": "image",
        "media.result": "succeeded",
    }
    event = cast(
        _EventLogRecord,
        next(
            record
            for record in caplog.records
            if getattr(record, "event_name", None) == "media.probe.completed"
        ),
    )
    assert event.context["kind"] == "image"
    assert event.context["result"] == "succeeded"
    assert "temporary_path" not in event.context


@pytest.mark.asyncio
async def test_ffprobe_tool_unavailable_is_observable_without_leaking_the_path(
    caplog: pytest.LogCaptureFixture,
) -> None:
    async def content() -> AsyncIterator[bytes]:
        yield ONE_PIXEL_PNG

    executable = "lanverse-private-missing-ffprobe"
    provider = configure_telemetry(
        service_name="lanverse-ffprobe-contract",
        environment="test",
    )
    exporter = InMemorySpanExporter()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    exporter.clear()
    labels = {"kind": "image", "result": "probe_tool_unavailable"}
    before = REGISTRY.get_sample_value("lanverse_media_probe_results_total", labels) or 0
    caplog.set_level(logging.WARNING, logger="lanverse.media.probe")

    with pytest.raises(MediaProbeError) as captured:
        await FfprobeMediaProbe(
            timeout_seconds=1,
            executable=executable,
        ).probe(content(), kind="image", mime_type="image/png")

    assert captured.value.code == "probe_tool_unavailable"
    assert REGISTRY.get_sample_value("lanverse_media_probe_results_total", labels) == (
        before + 1
    )
    span = next(
        span for span in exporter.get_finished_spans() if span.name == "media.ffprobe"
    )
    assert span.attributes == {
        "media.tool": "ffprobe",
        "media.kind": "image",
        "media.result": "probe_tool_unavailable",
    }
    event = cast(
        _EventLogRecord,
        next(
            record
            for record in caplog.records
            if getattr(record, "event_name", None) == "media.probe.failed"
        ),
    )
    assert event.context["error_code"] == "probe_tool_unavailable"
    assert executable not in str(vars(event))
