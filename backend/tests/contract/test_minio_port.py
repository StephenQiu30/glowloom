import os
from uuid import uuid4

import pytest

from app.core.config import Settings
from app.integrations.minio import MinioObjectStorage


@pytest.mark.asyncio
async def test_minio_round_trip_contract() -> None:
    if os.getenv("LANVERSE_RUN_MINIO_CONTRACT") != "1":
        pytest.skip("set LANVERSE_RUN_MINIO_CONTRACT=1 with the explicit MinIO profile running")
    settings = Settings()
    storage = MinioObjectStorage(
        settings.minio_endpoint,
        settings.minio_access_key,
        settings.minio_secret_key,
        settings.minio_bucket,
        secure=settings.minio_secure,
    )
    key = f"contract/{uuid4()}.txt"
    await storage.ensure_bucket()
    await storage.put(key, b"lanverse", "text/plain")
    try:
        size, content_type = await storage.stat(key)
        assert size == 8
        assert content_type == "text/plain"
        assert b"".join([chunk async for chunk in storage.stream(key)]) == b"lanverse"
        assert "X-Amz-Signature" in await storage.presign_download(key, 60)
        assert "X-Amz-Signature" in await storage.presign_upload(f"{key}.upload", 60)
    finally:
        await storage.delete(key)
