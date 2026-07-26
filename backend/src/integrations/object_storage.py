from __future__ import annotations

import asyncio
import hashlib
import re
from dataclasses import dataclass
from typing import Protocol
from uuid import UUID

OUTPUT_SLOT = re.compile(r"^(primary|mp4|srt|manifest|extra/[0-9]+)$")
EXTENSIONS = {
    "image/png": "png",
    "video/mp4": "mp4",
    "audio/wav": "wav",
    "audio/aac": "aac",
    "application/x-subrip": "srt",
    "application/json": "json",
}


class ObjectStoreUnavailable(RuntimeError):
    pass


class ObjectKeyConflict(RuntimeError):
    pass


@dataclass(frozen=True, slots=True)
class RemoteObject:
    byte_size: int
    sha256: str
    content_type: str


@dataclass(frozen=True, slots=True)
class StoredObject:
    bucket: str
    object_key: str
    byte_size: int
    sha256: str
    content_type: str


@dataclass(frozen=True, slots=True)
class ObjectLocation:
    bucket: str
    object_key: str


class ObjectTransport(Protocol):
    def stat(self, bucket: str, object_key: str) -> RemoteObject | None: ...

    def put(
        self,
        bucket: str,
        object_key: str,
        data: bytes,
        content_type: str,
        sha256: str,
    ) -> None: ...


class ObjectStore(Protocol):
    def invalid_location(
        self, episode_id: UUID, attempt_id: UUID, output_slot: str
    ) -> ObjectLocation: ...

    async def put(
        self,
        *,
        episode_id: UUID,
        attempt_id: UUID,
        output_slot: str,
        content_type: str,
        data: bytes,
    ) -> StoredObject: ...


class MinioObjectStore:
    def __init__(self, transport: ObjectTransport, *, bucket: str) -> None:
        if not bucket or bucket != bucket.strip():
            raise ValueError("bucket must be a non-empty trimmed value")
        self._transport = transport
        self._bucket = bucket

    def object_key(
        self,
        episode_id: UUID,
        attempt_id: UUID,
        output_slot: str,
        content_type: str,
    ) -> str:
        if OUTPUT_SLOT.fullmatch(output_slot) is None:
            raise ValueError("output slot is invalid")
        extension = EXTENSIONS.get(content_type)
        if extension is None:
            raise ValueError("content type is not supported")
        safe_slot = output_slot.replace("/", "-")
        return f"episodes/{episode_id}/attempts/{attempt_id}/{safe_slot}.{extension}"

    def invalid_location(
        self, episode_id: UUID, attempt_id: UUID, output_slot: str
    ) -> ObjectLocation:
        if OUTPUT_SLOT.fullmatch(output_slot) is None:
            raise ValueError("output slot is invalid")
        safe_slot = output_slot.replace("/", "-")
        key = f"episodes/{episode_id}/attempts/{attempt_id}/{safe_slot}.invalid"
        return ObjectLocation(self._bucket, key)

    async def put(
        self,
        *,
        episode_id: UUID,
        attempt_id: UUID,
        output_slot: str,
        content_type: str,
        data: bytes,
    ) -> StoredObject:
        object_key = self.object_key(episode_id, attempt_id, output_slot, content_type)
        sha256 = hashlib.sha256(data).hexdigest()
        expected = RemoteObject(len(data), sha256, content_type)
        existing = await self._stat(object_key)
        if existing is not None:
            self._assert_same(existing, expected)
        else:
            try:
                await asyncio.to_thread(
                    self._transport.put,
                    self._bucket,
                    object_key,
                    data,
                    content_type,
                    sha256,
                )
            except (ObjectStoreUnavailable, OSError) as error:
                raise ObjectStoreUnavailable("object upload failed") from error
            persisted = await self._stat(object_key)
            if persisted is None:
                raise ObjectStoreUnavailable("uploaded object cannot be read")
            self._assert_same(persisted, expected)
        return StoredObject(self._bucket, object_key, len(data), sha256, content_type)

    async def _stat(self, object_key: str) -> RemoteObject | None:
        try:
            return await asyncio.to_thread(self._transport.stat, self._bucket, object_key)
        except (ObjectStoreUnavailable, OSError) as error:
            raise ObjectStoreUnavailable("object lookup failed") from error

    @staticmethod
    def _assert_same(existing: RemoteObject, expected: RemoteObject) -> None:
        if existing != expected:
            raise ObjectKeyConflict("stable object key already contains different facts")
