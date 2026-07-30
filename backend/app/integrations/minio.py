from collections.abc import AsyncIterator
from datetime import timedelta
from functools import partial
from io import BytesIO
from typing import TypeVar

from anyio import CapacityLimiter
from anyio.to_thread import run_sync
from minio import Minio
from minio.error import S3Error

from app.modules.media.storage import StorageObjectNotFound, StorageUnavailable

T = TypeVar("T")


class MinioObjectStorage:
    def __init__(
        self,
        endpoint: str,
        access_key: str,
        secret_key: str,
        bucket: str,
        *,
        secure: bool,
        thread_limit: int = 4,
    ) -> None:
        self._client = Minio(endpoint, access_key=access_key, secret_key=secret_key, secure=secure)
        self._bucket = bucket
        self._limiter = CapacityLimiter(thread_limit)

    async def _run(self, function: partial[T]) -> T:
        return await run_sync(function, limiter=self._limiter)

    async def ensure_bucket(self) -> None:
        try:
            exists = await self._run(partial(self._client.bucket_exists, self._bucket))
            if not exists:
                await self._run(partial(self._client.make_bucket, self._bucket))
        except Exception as error:
            raise StorageUnavailable("object storage is unavailable") from error

    async def presign_upload(self, object_key: str, expires_seconds: int) -> str:
        try:
            result = await self._run(
                partial(
                    self._client.presigned_put_object,
                    self._bucket,
                    object_key,
                    timedelta(seconds=expires_seconds),
                )
            )
        except Exception as error:
            raise StorageUnavailable("object storage is unavailable") from error
        return str(result)

    async def presign_download(self, object_key: str, expires_seconds: int) -> str:
        try:
            result = await self._run(
                partial(
                    self._client.presigned_get_object,
                    self._bucket,
                    object_key,
                    timedelta(seconds=expires_seconds),
                )
            )
        except Exception as error:
            raise StorageUnavailable("object storage is unavailable") from error
        return str(result)

    async def stat(self, object_key: str) -> tuple[int, str | None]:
        try:
            result = await self._run(
                partial(self._client.stat_object, self._bucket, object_key)
            )
        except S3Error as error:
            if error.code in {"NoSuchKey", "NoSuchObject", "NotFound"}:
                raise StorageObjectNotFound("object does not exist") from error
            raise StorageUnavailable("object storage is unavailable") from error
        except Exception as error:
            raise StorageUnavailable("object storage is unavailable") from error
        if result.size is None:
            raise StorageUnavailable("object storage returned no object size")
        return result.size, result.content_type

    async def put(self, object_key: str, data: bytes, content_type: str) -> None:
        stream = BytesIO(data)
        try:
            await self._run(
                partial(
                    self._client.put_object,
                    self._bucket,
                    object_key,
                    stream,
                    len(data),
                    content_type,
                )
            )
        except Exception as error:
            raise StorageUnavailable("object storage is unavailable") from error

    async def stream(self, object_key: str) -> AsyncIterator[bytes]:
        try:
            response = await self._run(
                partial(self._client.get_object, self._bucket, object_key)
            )
        except S3Error as error:
            if error.code in {"NoSuchKey", "NoSuchObject", "NotFound"}:
                raise StorageObjectNotFound("object does not exist") from error
            raise StorageUnavailable("object storage is unavailable") from error
        except Exception as error:
            raise StorageUnavailable("object storage is unavailable") from error
        try:
            while True:
                try:
                    chunk = await self._run(partial(response.read, 1024 * 1024))
                except Exception as error:
                    raise StorageUnavailable("object storage is unavailable") from error
                if not chunk:
                    break
                yield chunk
        finally:
            await self._run(partial(response.close))
            await self._run(partial(response.release_conn))

    async def delete(self, object_key: str) -> None:
        try:
            await self._run(partial(self._client.remove_object, self._bucket, object_key))
        except Exception as error:
            raise StorageUnavailable("object storage is unavailable") from error
