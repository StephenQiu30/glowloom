from __future__ import annotations

from uuid import UUID

import pytest

from integrations.object_storage import (
    ObjectIntegrityError,
    MinioObjectStore,
    ObjectKeyConflict,
    ObjectStoreUnavailable,
    RemoteObject,
)

EPISODE_ID = UUID("11111111-1111-4111-8111-111111111111")
ATTEMPT_ID = UUID("22222222-2222-4222-8222-222222222222")


class FakeTransport:
    def __init__(self) -> None:
        self.objects: dict[tuple[str, str], RemoteObject] = {}
        self.contents: dict[tuple[str, str], bytes] = {}
        self.write_count = 0
        self.fail_next_put = False

    def stat(self, bucket: str, object_key: str) -> RemoteObject | None:
        return self.objects.get((bucket, object_key))

    def put(
        self,
        bucket: str,
        object_key: str,
        data: bytes,
        content_type: str,
        sha256: str,
    ) -> None:
        self.write_count += 1
        if self.fail_next_put:
            self.fail_next_put = False
            raise OSError("interrupted")
        identity = (bucket, object_key)
        self.contents[identity] = data
        self.objects[identity] = RemoteObject(
            byte_size=len(data),
            sha256=sha256,
            content_type=content_type,
        )

    def get(self, bucket: str, object_key: str) -> bytes:
        return self.contents[(bucket, object_key)]


@pytest.mark.asyncio
async def test_put_uses_stable_private_key_and_is_idempotent() -> None:
    transport = FakeTransport()
    store = MinioObjectStore(transport, bucket="lanverse")

    first = await store.put(
        episode_id=EPISODE_ID,
        attempt_id=ATTEMPT_ID,
        output_slot="primary",
        content_type="image/png",
        data=b"valid-png-bytes",
    )
    replay = await store.put(
        episode_id=EPISODE_ID,
        attempt_id=ATTEMPT_ID,
        output_slot="primary",
        content_type="image/png",
        data=b"valid-png-bytes",
    )

    assert first == replay
    assert first.bucket == "lanverse"
    assert first.object_key == (
        "episodes/11111111-1111-4111-8111-111111111111/"
        "attempts/22222222-2222-4222-8222-222222222222/primary.png"
    )
    assert first.byte_size == 15
    assert len(first.sha256) == 64
    assert transport.write_count == 1


@pytest.mark.asyncio
async def test_put_rejects_existing_object_with_different_facts() -> None:
    transport = FakeTransport()
    store = MinioObjectStore(transport, bucket="lanverse")
    object_key = store.object_key(
        episode_id=EPISODE_ID,
        attempt_id=ATTEMPT_ID,
        output_slot="primary",
        content_type="video/mp4",
    )
    transport.objects[("lanverse", object_key)] = RemoteObject(
        byte_size=3,
        sha256="b" * 64,
        content_type="video/mp4",
    )

    with pytest.raises(ObjectKeyConflict):
        await store.put(
            episode_id=EPISODE_ID,
            attempt_id=ATTEMPT_ID,
            output_slot="primary",
            content_type="video/mp4",
            data=b"new",
        )
    assert transport.write_count == 0


@pytest.mark.asyncio
async def test_interrupted_upload_can_retry_the_same_key() -> None:
    transport = FakeTransport()
    transport.fail_next_put = True
    store = MinioObjectStore(transport, bucket="lanverse")
    arguments = {
        "episode_id": EPISODE_ID,
        "attempt_id": ATTEMPT_ID,
        "output_slot": "extra/0",
        "content_type": "audio/wav",
        "data": b"wav-bytes",
    }

    with pytest.raises(ObjectStoreUnavailable):
        await store.put(**arguments)
    result = await store.put(**arguments)

    assert result.object_key.endswith("/extra-0.wav")
    assert transport.write_count == 2


def test_object_key_rejects_unknown_mime_and_unsafe_slots() -> None:
    store = MinioObjectStore(FakeTransport(), bucket="lanverse")

    with pytest.raises(ValueError, match="content type"):
        store.object_key(EPISODE_ID, ATTEMPT_ID, "primary", "text/html")
    with pytest.raises(ValueError, match="output slot"):
        store.object_key(EPISODE_ID, ATTEMPT_ID, "../escape", "image/png")


@pytest.mark.asyncio
async def test_read_verifies_private_location_size_and_digest() -> None:
    transport = FakeTransport()
    store = MinioObjectStore(transport, bucket="lanverse")
    stored = await store.put(
        episode_id=EPISODE_ID,
        attempt_id=ATTEMPT_ID,
        output_slot="primary",
        content_type="video/mp4",
        data=b"trusted-media",
    )

    assert await store.read(
        bucket=stored.bucket,
        object_key=stored.object_key,
        expected_sha256=stored.sha256,
        max_bytes=32,
    ) == b"trusted-media"

    transport.contents[(stored.bucket, stored.object_key)] = b"tampered-media"
    with pytest.raises(ObjectIntegrityError, match="digest"):
        await store.read(
            bucket=stored.bucket,
            object_key=stored.object_key,
            expected_sha256=stored.sha256,
            max_bytes=32,
        )

    with pytest.raises(ValueError, match="private bucket"):
        await store.read(
            bucket="another-bucket",
            object_key=stored.object_key,
            expected_sha256=stored.sha256,
            max_bytes=32,
        )
