from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Protocol


class StorageObjectNotFound(Exception):
    """The requested object does not exist in the selected private store."""


class StorageUnavailable(Exception):
    """The selected object store is temporarily unavailable."""


class ObjectStoragePort(Protocol):
    async def ensure_bucket(self) -> None: ...

    async def presign_upload(self, object_key: str, expires_seconds: int) -> str: ...

    async def presign_download(self, object_key: str, expires_seconds: int) -> str: ...

    async def stat(self, object_key: str) -> tuple[int, str | None]: ...

    async def put(self, object_key: str, data: bytes, content_type: str) -> None: ...

    def stream(self, object_key: str) -> AsyncIterator[bytes]: ...

    async def delete(self, object_key: str) -> None: ...


@dataclass(frozen=True, slots=True)
class MediaStorage:
    port: ObjectStoragePort
    profile: str
    bucket: str
