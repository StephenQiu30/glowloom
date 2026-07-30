import hashlib
import os
from typing import cast
from urllib.parse import unquote, urlparse
from uuid import UUID

import aio_pika
import httpx
import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings
from app.integrations.ffprobe import FfprobeMediaProbe
from app.integrations.minio import MinioObjectStorage
from app.integrations.rabbitmq import MEDIA_QUEUE, RabbitMQPublisher
from app.media_worker import process_incoming_message
from app.modules.media.models import MediaVersion
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production.models import Task
from tests.support.external_contracts import rabbitmq_contract_url
from tests.support.identity_builders import register_identity_response
from tests.support.media_fixtures import ONE_PIXEL_PNG


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_MEDIA_STACK_CONTRACT") != "1",
    reason="set LANVERSE_RUN_MEDIA_STACK_CONTRACT=1 with isolated RabbitMQ and MinIO",
)
@pytest.mark.asyncio
async def test_private_upload_reaches_ready_through_the_real_media_stack(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    rabbitmq_url = rabbitmq_contract_url()
    storage = MinioObjectStorage(
        test_settings.minio_endpoint,
        test_settings.minio_access_key,
        test_settings.minio_secret_key,
        test_settings.minio_bucket,
        secure=test_settings.minio_secure,
        thread_limit=test_settings.storage_thread_limit,
    )
    probe = FfprobeMediaProbe(
        timeout_seconds=test_settings.media_probe_timeout_seconds
    )
    publisher = RabbitMQPublisher(rabbitmq_url)
    observer = await aio_pika.connect_robust(rabbitmq_url, timeout=3)
    message: aio_pika.abc.AbstractIncomingMessage | None = None
    duplicate: aio_pika.abc.AbstractIncomingMessage | None = None
    object_key: str | None = None
    try:
        await storage.ensure_bucket()
        await publisher.connect()
        channel = await observer.channel()
        queue = await channel.declare_queue(MEDIA_QUEUE, durable=True)
        assert queue.declaration_result.message_count == 0

        identity = await register_identity_response(
            client, email="media-stack-contract@example.com"
        )
        assert identity.status_code == 201
        identity_data = identity.json()["data"]
        headers = {"authorization": f"Bearer {identity_data['access_token']}"}
        workspace_id = identity_data["workspace"]["id"]

        initialized = await client.post(
            "/api/v1/media/uploads",
            headers=headers,
            json={
                "workspace_id": workspace_id,
                "kind": "image",
                "filename": "stack-contract.png",
                "size_bytes": len(ONE_PIXEL_PNG),
                "mime_type": "image/png",
                "sha256": hashlib.sha256(ONE_PIXEL_PNG).hexdigest(),
                "idempotency_key": "media-stack-contract",
            },
        )
        assert initialized.status_code == 201
        initialization = initialized.json()["data"]
        upload_url = cast(str, initialization["upload"]["url"])
        path = unquote(urlparse(upload_url).path)
        prefix = f"/{test_settings.minio_bucket}/"
        assert path.startswith(prefix)
        object_key = path.removeprefix(prefix)
        async with httpx.AsyncClient() as external:
            uploaded = await external.put(
                upload_url,
                content=ONE_PIXEL_PNG,
                headers=initialization["upload"]["headers"],
            )
        assert uploaded.status_code == 200

        completed = await client.post(
            f"/api/v1/media/uploads/{initialization['upload_session']['id']}/complete",
            headers=headers,
            json={},
        )
        assert completed.status_code == 200
        completion = completed.json()["data"]
        task_id = UUID(completion["probe_task"]["id"])
        version_id = UUID(completion["version"]["id"])
        assert completion["version"]["probe_status"] == "pending"

        async with session_factory() as session:
            event = await session.scalar(
                select(OutboxEvent).where(OutboxEvent.aggregate_id == task_id)
            )
            assert event is not None
            envelope = envelope_from_event(event)

        await publisher.publish(envelope, "media.probe")
        message = await queue.get(timeout=3, fail=False)
        assert message is not None
        assert (
            await process_incoming_message(
                message,
                session_factory,
                storage=storage,
                probe=probe,
            )
            == "completed"
        )
        assert message.processed is True

        detail = await client.get(f"/api/v1/media/{version_id}", headers=headers)
        assert detail.status_code == 200
        ready = detail.json()["data"]
        assert ready["probe_status"] == "ready"
        assert ready["width"] == 1
        assert ready["height"] == 1
        assert ready["codec"] == "png"
        assert ready["container"] == "png_pipe"
        async with session_factory() as session:
            task = await session.get(Task, task_id)
            version = await session.get(MediaVersion, version_id)
            delivery = await session.scalar(select(InboxDelivery))
            assert task is not None and task.status == "succeeded"
            assert version is not None and version.probe_status == "ready"
            assert delivery is not None and delivery.status == "completed"

        await publisher.publish(envelope, "media.probe")
        duplicate = await queue.get(timeout=3, fail=False)
        assert duplicate is not None
        assert (
            await process_incoming_message(
                duplicate,
                session_factory,
                storage=storage,
                probe=probe,
            )
            == "duplicate"
        )
        assert duplicate.processed is True
        queue_state = await channel.declare_queue(MEDIA_QUEUE, durable=True, passive=True)
        assert queue_state.declaration_result.message_count == 0
        await channel.close()
    finally:
        if message is not None and not message.processed:
            await message.nack(requeue=True)
        if duplicate is not None and not duplicate.processed:
            await duplicate.nack(requeue=True)
        if object_key is not None:
            await storage.delete(object_key)
        await publisher.close()
        await observer.close()
