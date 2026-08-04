import hashlib
import hmac
from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Protocol


class StorageObjectNotFound(Exception):
    """The requested object does not exist in the selected private store."""


class StorageAccessDenied(Exception):
    """The selected object store rejected the configured identity or signature."""


class StorageIntegrityMismatch(Exception):
    """Stored bytes or metadata do not match the immutable media declaration."""


class StorageUnavailable(Exception):
    """The selected object store is temporarily unavailable."""


@dataclass(frozen=True, slots=True)
class StorageObjectMetadata:
    size_bytes: int
    content_type: str | None
    etag: str | None


class ObjectStoragePort(Protocol):
    async def ensure_bucket(self) -> None: ...

    async def presign_upload(self, object_key: str, expires_seconds: int) -> str: ...

    async def presign_download(self, object_key: str, expires_seconds: int) -> str: ...

    async def stat(self, object_key: str) -> StorageObjectMetadata: ...

    async def put(self, object_key: str, data: bytes, content_type: str) -> None: ...

    async def copy(self, source_key: str, target_key: str) -> None: ...

    def stream(self, object_key: str) -> AsyncIterator[bytes]: ...

    async def delete(self, object_key: str) -> None: ...


@dataclass(frozen=True, slots=True)
class MediaStorage:
    port: ObjectStoragePort
    profile: str
    bucket: str


def _normalized_content_type(value: str | None) -> str:
    return (value or "").split(";", 1)[0].strip().lower()


async def verify_object_integrity(
    storage: ObjectStoragePort,
    object_key: str,
    *,
    expected_size_bytes: int,
    expected_sha256: str,
    expected_content_type: str | None = None,
) -> StorageObjectMetadata:
    metadata = await storage.stat(object_key)
    if metadata.size_bytes != expected_size_bytes:
        raise StorageIntegrityMismatch("object size does not match its declaration")
    if expected_content_type is not None and _normalized_content_type(
        metadata.content_type
    ) != _normalized_content_type(expected_content_type):
        raise StorageIntegrityMismatch("object content type does not match its declaration")
    digest = hashlib.sha256()
    async for chunk in storage.stream(object_key):
        digest.update(chunk)
    if not hmac.compare_digest(digest.hexdigest(), expected_sha256.lower()):
        raise StorageIntegrityMismatch("object hash does not match its declaration")
    return metadata
