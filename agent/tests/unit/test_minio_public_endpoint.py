from urllib.parse import urlsplit

import pytest

from app.integrations.minio import MinioObjectStorage


@pytest.mark.asyncio
async def test_presigned_urls_use_the_browser_reachable_endpoint() -> None:
    storage = MinioObjectStorage(
        "minio:9000",
        "lanverse",
        "lanverse-development-only",
        "lanverse-media",
        secure=False,
        public_endpoint="127.0.0.1:59000",
        public_secure=False,
        region="us-east-1",
    )

    upload_url = await storage.presign_upload("scripts/example.md", 60)
    download_url = await storage.presign_download("scripts/example.md", 60)

    assert urlsplit(upload_url).netloc == "127.0.0.1:59000"
    assert urlsplit(download_url).netloc == "127.0.0.1:59000"
    assert urlsplit(upload_url).scheme == "http"
