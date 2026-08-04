import hashlib
import logging
import os
from collections.abc import AsyncGenerator
from typing import Protocol, cast
from urllib.parse import quote
from uuid import uuid4

import httpx
import pytest
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from prometheus_client import REGISTRY

from app.core.config import Settings
from app.core.telemetry import configure_telemetry
from app.integrations.minio import MinioObjectStorage
from app.modules.media.storage import (
    StorageAccessDenied,
    StorageIntegrityMismatch,
    StorageObjectNotFound,
    StorageUnavailable,
    verify_object_integrity,
)

pytestmark = pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_MINIO_CONTRACT") != "1",
    reason="set LANVERSE_RUN_MINIO_CONTRACT=1 with the explicit MinIO profile running",
)


class _EventLogRecord(Protocol):
    event_name: str
    context: dict[str, object]


def _storage(settings: Settings, *, operation_timeout_seconds: float = 3) -> MinioObjectStorage:
    return MinioObjectStorage(
        settings.minio_endpoint,
        settings.minio_access_key,
        settings.minio_secret_key,
        settings.minio_bucket,
        secure=settings.minio_secure,
        thread_limit=settings.storage_thread_limit,
        operation_timeout_seconds=operation_timeout_seconds,
    )


@pytest.mark.asyncio
async def test_private_minio_supports_the_complete_eight_operation_contract(
    caplog: pytest.LogCaptureFixture,
) -> None:
    settings = Settings()
    storage = _storage(settings)
    namespace = f"contract/{uuid4()}"
    source_key = f"{namespace}/source.txt"
    copied_key = f"{namespace}/copied.txt"
    upload_key = f"{namespace}/presigned-upload.txt"
    missing_key = f"{namespace}/missing.txt"
    keys = (source_key, copied_key, upload_key, missing_key)
    content = b"lanverse-private-storage"
    provider = configure_telemetry(
        service_name="lanverse-minio-contract",
        environment="test",
    )
    exporter = InMemorySpanExporter()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    exporter.clear()
    put_labels = {
        "storage_profile": "default",
        "operation": "put",
        "result": "succeeded",
    }
    put_before = REGISTRY.get_sample_value(
        "lanverse_storage_operations_total", put_labels
    ) or 0
    byte_labels = {"storage_profile": "default", "operation": "put"}
    bytes_before = REGISTRY.get_sample_value("lanverse_storage_bytes_total", byte_labels) or 0
    caplog.set_level(logging.INFO, logger="lanverse.storage")
    await storage.ensure_bucket()
    try:
        await storage.put(source_key, content, "text/plain")
        metadata = await storage.stat(source_key)
        assert metadata.size_bytes == len(content)
        assert metadata.content_type == "text/plain"
        assert metadata.etag
        assert b"".join([chunk async for chunk in storage.stream(source_key)]) == content

        await storage.copy(source_key, copied_key)
        copied = await verify_object_integrity(
            storage,
            copied_key,
            expected_size_bytes=len(content),
            expected_sha256=hashlib.sha256(content).hexdigest(),
            expected_content_type="text/plain",
        )
        assert copied.size_bytes == len(content)

        upload_url = await storage.presign_upload(upload_key, 60)
        download_url = await storage.presign_download(source_key, 60)
        async with httpx.AsyncClient(timeout=5) as external:
            uploaded = await external.put(
                upload_url,
                content=content,
                headers={"content-type": "text/plain"},
            )
            downloaded = await external.get(download_url)
            scheme = "https" if settings.minio_secure else "http"
            anonymous_url = (
                f"{scheme}://{settings.minio_endpoint}/{settings.minio_bucket}/"
                f"{quote(source_key, safe='/')}"
            )
            anonymous = await external.get(anonymous_url)
        assert uploaded.status_code == 200
        assert downloaded.status_code == 200
        assert downloaded.content == content
        assert anonymous.status_code in {401, 403}

        with pytest.raises(StorageObjectNotFound):
            await storage.copy(missing_key, f"{namespace}/missing-copy.txt")
        await storage.delete(upload_key)
        await storage.delete(upload_key)
        with pytest.raises(StorageObjectNotFound):
            await storage.stat(upload_key)
    finally:
        for key in keys:
            await storage.delete(key)

    assert REGISTRY.get_sample_value("lanverse_storage_operations_total", put_labels) == (
        put_before + 1
    )
    assert REGISTRY.get_sample_value("lanverse_storage_bytes_total", byte_labels) == (
        bytes_before + len(content)
    )
    spans = [span for span in exporter.get_finished_spans() if span.name == "storage.minio"]
    span_operations: set[str] = set()
    for span in spans:
        assert span.attributes is not None
        span_operations.add(str(span.attributes["storage.operation"]))
    assert span_operations == {
        "ensure_bucket",
        "presign_upload",
        "presign_download",
        "stat",
        "put",
        "copy",
        "stream",
        "delete",
    }
    for span in spans:
        assert span.attributes is not None
        assert set(span.attributes) == {
            "storage.system",
            "storage.profile",
            "storage.operation",
            "storage.result",
        }
        rendered = str(span.attributes)
        assert namespace not in rendered
        assert settings.minio_endpoint not in rendered
        assert settings.minio_bucket not in rendered
    storage_events = [
        cast(_EventLogRecord, record)
        for record in caplog.records
        if str(getattr(record, "event_name", "")).startswith("storage.operation.")
    ]
    assert storage_events
    rendered_events = str([vars(record) for record in storage_events])
    assert namespace not in rendered_events
    assert settings.minio_endpoint not in rendered_events
    assert settings.minio_bucket not in rendered_events


@pytest.mark.asyncio
async def test_minio_multipart_etag_is_not_the_platform_content_hash() -> None:
    settings = Settings()
    storage = _storage(settings)
    key = f"contract/{uuid4()}/multipart.bin"
    content = (b"lanverse-multipart-contract-" * 210_000)[: 5 * 1024 * 1024 + 257]
    expected_sha256 = hashlib.sha256(content).hexdigest()
    await storage.ensure_bucket()
    try:
        await storage.put(key, content, "application/octet-stream")
        metadata = await storage.stat(key)
        assert metadata.size_bytes == len(content)
        assert metadata.etag is not None and "-" in metadata.etag
        assert metadata.etag != expected_sha256
        verified = await verify_object_integrity(
            storage,
            key,
            expected_size_bytes=len(content),
            expected_sha256=expected_sha256,
            expected_content_type="application/octet-stream",
        )
        assert verified == metadata
        with pytest.raises(StorageIntegrityMismatch):
            await verify_object_integrity(
                storage,
                key,
                expected_size_bytes=len(content),
                expected_sha256="0" * 64,
                expected_content_type="application/octet-stream",
            )
    finally:
        await storage.delete(key)


@pytest.mark.asyncio
async def test_partially_consumed_minio_stream_does_not_report_successful_bytes() -> None:
    settings = Settings()
    storage = _storage(settings)
    key = f"contract/{uuid4()}/partial-stream.bin"
    content = b"x" * (1024 * 1024 + 17)
    byte_labels = {"storage_profile": "default", "operation": "stream"}
    succeeded_labels = {
        **byte_labels,
        "result": "succeeded",
    }
    cancelled_labels = {
        **byte_labels,
        "result": "cancelled",
    }
    await storage.ensure_bucket()
    try:
        await storage.put(key, content, "application/octet-stream")
        bytes_before = (
            REGISTRY.get_sample_value("lanverse_storage_bytes_total", byte_labels) or 0
        )
        succeeded_before = (
            REGISTRY.get_sample_value(
                "lanverse_storage_operations_total", succeeded_labels
            )
            or 0
        )
        cancelled_before = (
            REGISTRY.get_sample_value(
                "lanverse_storage_operations_total", cancelled_labels
            )
            or 0
        )

        stream = cast(AsyncGenerator[bytes, None], storage.stream(key))
        first_chunk = await anext(stream)
        assert first_chunk
        await stream.aclose()

        assert (
            REGISTRY.get_sample_value("lanverse_storage_bytes_total", byte_labels)
            == bytes_before
        )
        assert (
            REGISTRY.get_sample_value(
                "lanverse_storage_operations_total", succeeded_labels
            )
            == succeeded_before
        )
        assert (
            REGISTRY.get_sample_value(
                "lanverse_storage_operations_total", cancelled_labels
            )
            == cancelled_before + 1
        )
    finally:
        await storage.delete(key)


@pytest.mark.asyncio
async def test_minio_maps_access_denied_and_unavailable_without_sdk_errors(
    caplog: pytest.LogCaptureFixture,
) -> None:
    settings = Settings()
    provider = configure_telemetry(
        service_name="lanverse-minio-contract",
        environment="test",
    )
    exporter = InMemorySpanExporter()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    exporter.clear()
    caplog.set_level(logging.WARNING, logger="lanverse.storage")
    denied = MinioObjectStorage(
        settings.minio_endpoint,
        f"invalid-{uuid4()}",
        "invalid-contract-secret",
        settings.minio_bucket,
        secure=settings.minio_secure,
        operation_timeout_seconds=1,
    )
    with pytest.raises(StorageAccessDenied, match="object storage access was denied"):
        await denied.ensure_bucket()

    unavailable = MinioObjectStorage(
        "127.0.0.1:1",
        "contract-access-key",
        "contract-secret-key",
        settings.minio_bucket,
        secure=False,
        operation_timeout_seconds=0.2,
    )
    with pytest.raises(StorageUnavailable, match="object storage is unavailable"):
        await unavailable.ensure_bucket()

    results: list[str] = []
    for span in exporter.get_finished_spans():
        if span.name != "storage.minio":
            continue
        assert span.attributes is not None
        results.append(str(span.attributes["storage.result"]))
    assert results == ["access_denied", "unavailable"]
    events = [
        cast(_EventLogRecord, record)
        for record in caplog.records
        if getattr(record, "event_name", None) == "storage.operation.failed"
    ]
    assert [record.context["error_code"] for record in events] == [
        "access_denied",
        "unavailable",
    ]
    rendered = str([vars(record) for record in events])
    assert "invalid-contract-secret" not in rendered
    assert "127.0.0.1:1" not in rendered


@pytest.mark.asyncio
async def test_minio_rejects_invalid_keys_and_expiry_before_network_access() -> None:
    settings = Settings()
    storage = MinioObjectStorage(
        "127.0.0.1:1",
        "contract-access-key",
        "contract-secret-key",
        settings.minio_bucket,
        secure=False,
        operation_timeout_seconds=0.2,
    )
    for invalid_key in ("", "/absolute", "a//b", "a/../b", "a/./b", "a\nb"):
        with pytest.raises(ValueError, match="object key"):
            await storage.stat(invalid_key)
    for invalid_expiry in (0, 604801):
        with pytest.raises(ValueError, match="expiry"):
            await storage.presign_download("contract/valid.txt", invalid_expiry)
