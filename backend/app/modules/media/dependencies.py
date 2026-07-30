from typing import Annotated

from fastapi import Depends

from app.core.auth import get_request_settings
from app.core.config import Settings
from app.integrations.minio import MinioObjectStorage
from app.modules.media.storage import MediaStorage


def get_media_storage(
    settings: Annotated[Settings, Depends(get_request_settings)],
) -> MediaStorage:
    return MediaStorage(
        port=MinioObjectStorage(
            settings.minio_endpoint,
            settings.minio_access_key,
            settings.minio_secret_key,
            settings.minio_bucket,
            secure=settings.minio_secure,
            thread_limit=settings.storage_thread_limit,
        ),
        profile="default",
        bucket=settings.minio_bucket,
    )
