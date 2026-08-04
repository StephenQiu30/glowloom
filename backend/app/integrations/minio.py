import json
from collections.abc import AsyncIterator, Callable
from contextlib import suppress
from datetime import timedelta
from functools import partial
from io import BytesIO
from typing import NoReturn, TypeVar, cast

from anyio import CapacityLimiter, fail_after
from anyio.to_thread import run_sync
from minio import Minio
from minio.commonconfig import CopySource
from minio.error import S3Error

from app.modules.media.storage import (
    StorageAccessDenied,
    StorageObjectMetadata,
    StorageObjectNotFound,
    StorageUnavailable,
)

T = TypeVar("T")

_ACCESS_DENIED_CODES = {
    "AccessDenied",
    "AuthorizationHeaderMalformed",
    "ExpiredToken",
    "InvalidAccessKeyId",
    "InvalidToken",
    "SignatureDoesNotMatch",
}
_NOT_FOUND_CODES = {"NoSuchKey", "NoSuchObject", "NotFound"}
_MAX_OBJECT_KEY_LENGTH = 1024
_MAX_PRESIGN_EXPIRY_SECONDS = 7 * 24 * 60 * 60


def _validate_object_key(object_key: str) -> None:
    segments = object_key.split("/")
    if (
        not object_key
        or len(object_key) > _MAX_OBJECT_KEY_LENGTH
        or object_key.startswith("/")
        or any(segment in {"", ".", ".."} for segment in segments)
        or any(ord(character) < 32 or ord(character) == 127 for character in object_key)
    ):
        raise ValueError("object key is invalid")


def _validate_expiry(expires_seconds: int) -> None:
    if (
        isinstance(expires_seconds, bool)
        or expires_seconds < 1
        or expires_seconds > _MAX_PRESIGN_EXPIRY_SECONDS
    ):
        raise ValueError("presigned URL expiry is invalid")


def _raise_storage_error(error: BaseException) -> NoReturn:
    if isinstance(error, (StorageAccessDenied, StorageObjectNotFound, StorageUnavailable)):
        raise error
    if isinstance(error, S3Error):
        if error.code in _NOT_FOUND_CODES:
            raise StorageObjectNotFound("object does not exist") from error
        if error.code in _ACCESS_DENIED_CODES:
            raise StorageAccessDenied("object storage access was denied") from error
    raise StorageUnavailable("object storage is unavailable") from error


def _policy_allows_anonymous_read(raw_policy: str) -> bool:
    try:
        parsed = cast(object, json.loads(raw_policy))
    except (TypeError, ValueError) as error:
        raise StorageUnavailable("object storage is unavailable") from error
    if not isinstance(parsed, dict):
        raise StorageUnavailable("object storage is unavailable")
    policy = cast(dict[str, object], parsed)
    raw_statements = policy.get("Statement", [])
    if isinstance(raw_statements, dict):
        statements: list[object] = [raw_statements]
    elif isinstance(raw_statements, list):
        statements = cast(list[object], raw_statements)
    else:
        raise StorageUnavailable("object storage is unavailable")
    for raw_statement in statements:
        if not isinstance(raw_statement, dict):
            continue
        statement = cast(dict[str, object], raw_statement)
        if statement.get("Effect") != "Allow":
            continue
        principal = statement.get("Principal")
        is_anonymous = principal == "*" or (
            isinstance(principal, dict)
            and cast(dict[str, object], principal).get("AWS") == "*"
        )
        raw_actions = statement.get("Action", [])
        if isinstance(raw_actions, str):
            actions = [raw_actions]
        elif isinstance(raw_actions, list):
            actions = [
                action
                for action in cast(list[object], raw_actions)
                if isinstance(action, str)
            ]
        else:
            actions = []
        if is_anonymous and any(
            action in {"*", "s3:*", "s3:GetObject"} for action in actions
        ):
            return True
    return False


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
        operation_timeout_seconds: float = 3,
    ) -> None:
        self._client = Minio(
            endpoint,
            access_key=access_key,
            secret_key=secret_key,
            secure=secure,
        )
        self._bucket = bucket
        self._limiter = CapacityLimiter(thread_limit)
        self._operation_timeout_seconds = operation_timeout_seconds

    async def _run(self, function: Callable[[], T]) -> T:
        try:
            with fail_after(self._operation_timeout_seconds):
                return await run_sync(
                    function,
                    limiter=self._limiter,
                    abandon_on_cancel=True,
                )
        except TimeoutError as error:
            raise StorageUnavailable("object storage is unavailable") from error

    async def ensure_bucket(self) -> None:
        try:
            exists = await self._run(partial(self._client.bucket_exists, self._bucket))
            if not exists:
                await self._run(partial(self._client.make_bucket, self._bucket))
            try:
                policy = await self._run(
                    partial(self._client.get_bucket_policy, self._bucket)
                )
            except S3Error as error:
                if error.code == "NoSuchBucketPolicy":
                    return
                raise
            if _policy_allows_anonymous_read(policy):
                raise StorageAccessDenied("object storage bucket must be private")
        except Exception as error:
            _raise_storage_error(error)

    async def presign_upload(self, object_key: str, expires_seconds: int) -> str:
        _validate_object_key(object_key)
        _validate_expiry(expires_seconds)
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
            _raise_storage_error(error)
        return str(result)

    async def presign_download(self, object_key: str, expires_seconds: int) -> str:
        _validate_object_key(object_key)
        _validate_expiry(expires_seconds)
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
            _raise_storage_error(error)
        return str(result)

    async def stat(self, object_key: str) -> StorageObjectMetadata:
        _validate_object_key(object_key)
        try:
            result = await self._run(
                partial(self._client.stat_object, self._bucket, object_key)
            )
        except Exception as error:
            _raise_storage_error(error)
        if result.size is None:
            raise StorageUnavailable("object storage returned no object size")
        return StorageObjectMetadata(
            size_bytes=result.size,
            content_type=result.content_type,
            etag=result.etag,
        )

    async def put(self, object_key: str, data: bytes, content_type: str) -> None:
        _validate_object_key(object_key)
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
            _raise_storage_error(error)

    async def copy(self, source_key: str, target_key: str) -> None:
        _validate_object_key(source_key)
        _validate_object_key(target_key)
        try:
            await self._run(
                partial(
                    self._client.copy_object,
                    self._bucket,
                    target_key,
                    CopySource(self._bucket, source_key),
                )
            )
        except Exception as error:
            _raise_storage_error(error)

    async def stream(self, object_key: str) -> AsyncIterator[bytes]:
        _validate_object_key(object_key)
        try:
            response = await self._run(
                partial(self._client.get_object, self._bucket, object_key)
            )
        except Exception as error:
            _raise_storage_error(error)
        try:
            while True:
                try:
                    chunk = await self._run(partial(response.read, 1024 * 1024))
                except Exception as error:
                    _raise_storage_error(error)
                if not chunk:
                    break
                yield chunk
        finally:
            with suppress(StorageUnavailable):
                await self._run(partial(response.close))
            with suppress(StorageUnavailable):
                await self._run(partial(response.release_conn))

    async def delete(self, object_key: str) -> None:
        _validate_object_key(object_key)
        try:
            await self._run(partial(self._client.remove_object, self._bucket, object_key))
        except Exception as error:
            _raise_storage_error(error)
