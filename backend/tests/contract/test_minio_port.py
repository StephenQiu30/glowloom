import hashlib
import os
from urllib.parse import quote
from uuid import uuid4

import httpx
import pytest

from app.core.config import Settings
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
async def test_private_minio_supports_the_complete_eight_operation_contract() -> None:
    settings = Settings()
    storage = _storage(settings)
    namespace = f"contract/{uuid4()}"
    source_key = f"{namespace}/source.txt"
    copied_key = f"{namespace}/copied.txt"
    upload_key = f"{namespace}/presigned-upload.txt"
    missing_key = f"{namespace}/missing.txt"
    keys = (source_key, copied_key, upload_key, missing_key)
    content = b"lanverse-private-storage"
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
async def test_minio_maps_access_denied_and_unavailable_without_sdk_errors() -> None:
    settings = Settings()
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
