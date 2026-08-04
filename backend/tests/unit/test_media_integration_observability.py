from collections.abc import AsyncIterator

import pytest
from prometheus_client import generate_latest

from app.integrations import ffprobe as ffprobe_module
from app.integrations import minio as minio_module
from app.integrations.ffprobe import FfprobeMediaProbe
from app.integrations.minio import MinioObjectStorage
from app.modules.media import MediaProbeError, metrics
from tests.support.media_fixtures import ONE_PIXEL_PNG


class _BrokenMetric:
    def labels(self, **_: str) -> None:
        raise RuntimeError("telemetry backend failed")


def test_storage_and_probe_metric_labels_are_bounded() -> None:
    hostile_value = "tenant/object/attacker-controlled-9f6297b8"

    assert metrics.storage_profile_label("default") == "default"
    assert metrics.storage_profile_label(hostile_value) == "unregistered"
    assert metrics.storage_operation_label("put") == "put"
    assert metrics.storage_operation_label(hostile_value) == "unregistered"
    assert metrics.storage_result_label("access_denied") == "access_denied"
    assert metrics.storage_result_label(hostile_value) == "unregistered"
    assert metrics.media_kind_label("image") == "image"
    assert metrics.media_kind_label(hostile_value) == "unregistered"
    assert metrics.media_probe_result_label("probe_timeout") == "probe_timeout"
    assert metrics.media_probe_result_label(hostile_value) == "unregistered"

    metrics.observe_storage_operation(
        storage_profile=hostile_value,
        operation=hostile_value,
        result=hostile_value,
        duration_seconds=-1,
        bytes_processed=17,
    )
    metrics.observe_media_probe(
        kind=hostile_value,
        result=hostile_value,
        duration_seconds=-1,
    )

    rendered = generate_latest().decode("utf-8")
    assert (
        'lanverse_storage_operations_total{operation="unregistered",'
        'result="unregistered",storage_profile="unregistered"}'
    ) in rendered
    assert (
        'lanverse_media_probe_results_total{kind="unregistered",'
        'result="unregistered"}'
    ) in rendered
    assert hostile_value not in rendered


def test_metric_export_failures_do_not_escape_observers(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(metrics, "STORAGE_OPERATIONS", _BrokenMetric())
    monkeypatch.setattr(metrics, "STORAGE_OPERATION_DURATION", _BrokenMetric())
    monkeypatch.setattr(metrics, "STORAGE_BYTES", _BrokenMetric())
    monkeypatch.setattr(metrics, "MEDIA_PROBE_RESULTS", _BrokenMetric())
    monkeypatch.setattr(metrics, "MEDIA_PROBE_DURATION", _BrokenMetric())

    metrics.observe_storage_operation(
        storage_profile="default",
        operation="put",
        result="succeeded",
        duration_seconds=0.1,
        bytes_processed=3,
    )
    metrics.observe_media_probe(
        kind="image",
        result="succeeded",
        duration_seconds=0.1,
    )


@pytest.mark.asyncio
async def test_logging_failures_do_not_replace_storage_or_probe_results(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    def fail_log(*_args: object, **_kwargs: object) -> None:
        raise RuntimeError("logging backend failed")

    async def content() -> AsyncIterator[bytes]:
        yield ONE_PIXEL_PNG

    monkeypatch.setattr(minio_module, "log_event", fail_log)
    storage = MinioObjectStorage(
        "127.0.0.1:1",
        "contract-access-key",
        "contract-secret-key",
        "contract-bucket",
        secure=False,
        operation_timeout_seconds=0.1,
    )
    with pytest.raises(ValueError, match="object key"):
        await storage.stat("")

    monkeypatch.setattr(ffprobe_module, "log_event", fail_log)
    with pytest.raises(MediaProbeError) as captured:
        await FfprobeMediaProbe(
            timeout_seconds=1,
            executable="lanverse-missing-ffprobe-for-log-failure",
        ).probe(content(), kind="image", mime_type="image/png")
    assert captured.value.code == "probe_tool_unavailable"
