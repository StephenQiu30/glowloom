import hashlib
import os
from typing import cast
from urllib.parse import unquote, urlparse

import httpx
import pytest

from app.core.config import Settings
from app.integrations.minio import MinioObjectStorage
from tests.support.identity_builders import register_identity_response


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_MINIO_CONTRACT") != "1",
    reason="set LANVERSE_RUN_MINIO_CONTRACT=1 with the explicit MinIO profile running",
)
@pytest.mark.asyncio
async def test_media_api_completes_a_real_private_minio_upload(
    client: httpx.AsyncClient,
    test_settings: Settings,
) -> None:
    storage = MinioObjectStorage(
        test_settings.minio_endpoint,
        test_settings.minio_access_key,
        test_settings.minio_secret_key,
        test_settings.minio_bucket,
        secure=test_settings.minio_secure,
        thread_limit=test_settings.storage_thread_limit,
    )
    await storage.ensure_bucket()
    identity = await register_identity_response(
        client, email="media-minio-contract@example.com"
    )
    assert identity.status_code == 201
    identity_data = identity.json()["data"]
    headers = {"authorization": f"Bearer {identity_data['access_token']}"}
    workspace_id = identity_data["workspace"]["id"]
    content = b"real-minio-private-media-contract"
    initialized = await client.post(
        "/api/v1/media/uploads",
        headers=headers,
        json={
            "workspace_id": workspace_id,
            "kind": "image",
            "filename": "contract.png",
            "size_bytes": len(content),
            "mime_type": "image/png",
            "sha256": hashlib.sha256(content).hexdigest(),
            "idempotency_key": "real-minio-media-contract",
        },
    )
    assert initialized.status_code == 201
    result = initialized.json()["data"]
    upload_url = cast(str, result["upload"]["url"])
    path = unquote(urlparse(upload_url).path)
    prefix = f"/{test_settings.minio_bucket}/"
    assert path.startswith(prefix)
    object_key = path.removeprefix(prefix)
    try:
        async with httpx.AsyncClient() as external:
            uploaded = await external.put(
                upload_url,
                content=content,
                headers=result["upload"]["headers"],
            )
        assert uploaded.status_code == 200

        completed = await client.post(
            f"/api/v1/media/uploads/{result['upload_session']['id']}/complete",
            headers=headers,
            json={},
        )
        assert completed.status_code == 200
        version = completed.json()["data"]["version"]
        assert version["sha256"] == hashlib.sha256(content).hexdigest()
        assert version["size_bytes"] == len(content)

        access = await client.post(
            f"/api/v1/media/{version['id']}/access",
            headers=headers,
            json={"purpose": "download"},
        )
        assert access.status_code == 200
        async with httpx.AsyncClient() as external:
            downloaded = await external.get(access.json()["data"]["url"])
        assert downloaded.status_code == 200
        assert downloaded.content == content
    finally:
        await storage.delete(object_key)
