import asyncio
import json
import logging
import os
import tempfile
import time
from collections.abc import AsyncIterator
from contextlib import contextmanager
from functools import partial
from pathlib import Path
from typing import Any, cast

from anyio import fail_after, open_file, run_process
from anyio.to_thread import run_sync
from opentelemetry.trace import SpanKind, Status, StatusCode

from app.core.logging import log_event
from app.core.telemetry import span_identifiers, start_span
from app.modules.media import MediaProbeError, MediaProbeResult
from app.modules.media.metrics import (
    media_kind_label,
    media_probe_result_label,
    observe_media_probe,
)

_LOGGER = logging.getLogger("lanverse.media.probe")


def _probe_observation_result(error: BaseException) -> str:
    if isinstance(error, MediaProbeError):
        return media_probe_result_label(error.code)
    if isinstance(error, (asyncio.CancelledError, GeneratorExit)):
        return "cancelled"
    return "failed"


def _record_probe_observation(
    *,
    trace_id: str,
    span_id: str,
    kind: str,
    result: str,
    duration_seconds: float,
) -> None:
    observe_media_probe(
        kind=kind,
        result=result,
        duration_seconds=duration_seconds,
    )
    event_name = "media.probe.completed" if result == "succeeded" else "media.probe.failed"
    attributes: dict[str, object] = {
        "trace_id": trace_id,
        "span_id": span_id,
        "kind": kind,
        "result": result,
        "duration_ms": round(max(duration_seconds, 0) * 1000, 3),
    }
    if result != "succeeded":
        attributes["error_code"] = result
    try:
        log_event(
            _LOGGER,
            logging.INFO if result == "succeeded" else logging.WARNING,
            event_name,
            "media probe completed" if result == "succeeded" else "media probe failed",
            **attributes,
        )
    except Exception:
        pass


@contextmanager
def _observe_probe(kind: str):
    normalized_kind = media_kind_label(kind)
    started = time.perf_counter()
    with start_span(
        "media.ffprobe",
        kind=SpanKind.CLIENT,
        attributes={"media.tool": "ffprobe", "media.kind": normalized_kind},
    ) as span:
        trace_id, span_id = span_identifiers(span)
        try:
            yield
        except BaseException as error:
            result = _probe_observation_result(error)
            span.set_attribute("media.result", result)
            span.set_status(Status(StatusCode.ERROR, result))
            _record_probe_observation(
                trace_id=trace_id,
                span_id=span_id,
                kind=normalized_kind,
                result=result,
                duration_seconds=time.perf_counter() - started,
            )
            raise
        else:
            span.set_attribute("media.result", "succeeded")
            _record_probe_observation(
                trace_id=trace_id,
                span_id=span_id,
                kind=normalized_kind,
                result="succeeded",
                duration_seconds=time.perf_counter() - started,
            )


class FfprobeMediaProbe:
    def __init__(self, *, timeout_seconds: int, executable: str = "ffprobe") -> None:
        self._timeout_seconds = timeout_seconds
        self._executable = executable

    async def probe(
        self,
        content: AsyncIterator[bytes],
        *,
        kind: str,
        mime_type: str,
    ) -> MediaProbeResult:
        with _observe_probe(kind):
            suffix = Path("placeholder" + _suffix_for_mime(mime_type)).suffix
            descriptor, path = await run_sync(partial(tempfile.mkstemp, suffix=suffix))
            await run_sync(partial(os.close, descriptor))
            try:
                async with await open_file(path, "wb") as target:
                    async for chunk in content:
                        await target.write(chunk)
                try:
                    with fail_after(self._timeout_seconds):
                        completed = await run_process(
                            [
                                self._executable,
                                "-v",
                                "error",
                                "-print_format",
                                "json",
                                "-show_format",
                                "-show_streams",
                                path,
                            ],
                            check=False,
                        )
                except TimeoutError as error:
                    raise MediaProbeError(
                        "probe_timeout", "Media inspection exceeded its time limit"
                    ) from error
                except FileNotFoundError as error:
                    raise MediaProbeError(
                        "probe_tool_unavailable", "Media inspection tool is unavailable"
                    ) from error
                if completed.returncode != 0:
                    raise MediaProbeError(
                        "unsupported_media", "Unable to inspect the uploaded media"
                    )
                try:
                    payload = cast(dict[str, Any], json.loads(completed.stdout))
                except (TypeError, ValueError, UnicodeDecodeError) as error:
                    raise MediaProbeError(
                        "invalid_probe_output", "Media inspection returned invalid metadata"
                    ) from error
                return _probe_result(payload, kind=kind)
            finally:
                await run_sync(partial(Path(path).unlink, missing_ok=True))


def _suffix_for_mime(mime_type: str) -> str:
    return {
        "image/jpeg": ".jpg",
        "image/png": ".png",
        "image/webp": ".webp",
        "video/mp4": ".mp4",
        "video/quicktime": ".mov",
        "video/webm": ".webm",
        "audio/mpeg": ".mp3",
        "audio/wav": ".wav",
        "audio/x-wav": ".wav",
        "audio/mp4": ".m4a",
        "audio/ogg": ".ogg",
        "text/vtt": ".vtt",
        "application/x-subrip": ".srt",
    }.get(mime_type, ".bin")


def _positive_int(value: object) -> int | None:
    return value if isinstance(value, int) and value > 0 else None


def _duration_ms(value: object) -> int | None:
    try:
        duration = float(str(value))
    except (TypeError, ValueError):
        return None
    return round(duration * 1000) if duration >= 0 else None


def _probe_result(payload: dict[str, Any], *, kind: str) -> MediaProbeResult:
    streams = payload.get("streams")
    stream_items = (
        [
            cast(dict[str, Any], item)
            for item in cast(list[object], streams)
            if isinstance(item, dict)
        ]
        if isinstance(streams, list)
        else []
    )
    visual = next((item for item in stream_items if item.get("codec_type") == "video"), None)
    audio = next((item for item in stream_items if item.get("codec_type") == "audio"), None)
    primary = visual if kind in {"image", "video"} else audio
    if primary is None and kind in {"image", "video", "audio"}:
        raise MediaProbeError("unsupported_media", "Media does not contain the expected stream")
    format_payload = payload.get("format")
    media_format = cast(dict[str, Any], format_payload) if isinstance(format_payload, dict) else {}
    duration = _duration_ms(media_format.get("duration"))
    if duration is None and primary is not None:
        duration = _duration_ms(primary.get("duration"))
    width = _positive_int(visual.get("width")) if visual is not None else None
    height = _positive_int(visual.get("height")) if visual is not None else None
    if kind == "image" and (width is None or height is None):
        raise MediaProbeError("invalid_image_metadata", "Image dimensions could not be inspected")
    codec_value = primary.get("codec_name") if primary is not None else None
    container_value = media_format.get("format_name")
    return MediaProbeResult(
        width=width,
        height=height,
        duration_ms=duration,
        codec=codec_value if isinstance(codec_value, str) else None,
        container=container_value if isinstance(container_value, str) else None,
    )
