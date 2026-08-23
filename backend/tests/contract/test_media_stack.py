import hashlib
import os
from datetime import UTC, datetime, timedelta
from typing import cast
from urllib.parse import unquote, urlparse
from uuid import UUID

import aio_pika
import httpx
import pytest
from opentelemetry.sdk.trace.export import SimpleSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings
from app.core.telemetry import configure_telemetry
from app.integrations.ffprobe import FfprobeMediaProbe
from app.integrations.minio import MinioObjectStorage
from app.integrations.rabbitmq import MEDIA_QUEUE, RabbitMQPublisher
from app.media_worker import process_incoming_message
from app.modules.media.models import MediaLocation, MediaVersion, UploadSession
from app.modules.media.storage import StorageObjectNotFound
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production.models import Task
from app.modules.scheduling.dispatcher import dispatch_due_schedules
from app.modules.scheduling.models import Schedule, ScheduleFire
from tests.support.external_contracts import rabbitmq_contract_url
from tests.support.identity_builders import register_identity_response
from tests.support.media_fixtures import ONE_PIXEL_PNG


async def _assert_object_metadata(
    storage: MinioObjectStorage,
    object_key: str,
    *,
    size_bytes: int,
    content_type: str,
) -> None:
    metadata = await storage.stat(object_key)
    assert metadata.size_bytes == size_bytes
    assert metadata.content_type == content_type
    assert metadata.etag


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
    provider = configure_telemetry(
        service_name="lanverse-media-stack-contract",
        environment="test",
    )
    exporter = InMemorySpanExporter()
    provider.add_span_processor(SimpleSpanProcessor(exporter))
    exporter.clear()
    storage = MinioObjectStorage(
        test_settings.minio_endpoint,
        test_settings.minio_access_key,
        test_settings.minio_secret_key,
        test_settings.minio_bucket,
        secure=test_settings.minio_secure,
        thread_limit=test_settings.storage_thread_limit,
    )
    probe = FfprobeMediaProbe(timeout_seconds=test_settings.media_probe_timeout_seconds)
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

        spans = exporter.get_finished_spans()
        consumer_span = next(span for span in spans if span.name == "messaging.message.consume")
        probe_span = next(span for span in spans if span.name == "media.ffprobe")
        stream_spans = [
            span
            for span in spans
            if span.name == "storage.minio"
            and span.attributes is not None
            and span.attributes.get("storage.operation") == "stream"
        ]
        assert consumer_span.context is not None
        assert probe_span.context is not None and probe_span.parent is not None
        assert probe_span.context.trace_id == consumer_span.context.trace_id
        assert probe_span.parent.span_id == consumer_span.context.span_id
        stream_span = next(
            span
            for span in stream_spans
            if span.parent is not None and span.parent.span_id == probe_span.context.span_id
        )
        assert stream_span.context is not None and stream_span.parent is not None
        assert stream_span.context.trace_id == consumer_span.context.trace_id
        assert stream_span.parent.span_id == probe_span.context.span_id

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


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_MEDIA_STACK_CONTRACT") != "1",
    reason="set LANVERSE_RUN_MEDIA_STACK_CONTRACT=1 with isolated RabbitMQ and MinIO",
)
@pytest.mark.asyncio
async def test_expired_upload_is_dispatched_and_deleted_through_the_real_media_stack(
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
    probe = FfprobeMediaProbe(timeout_seconds=test_settings.media_probe_timeout_seconds)
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
            client, email="media-expiration-contract@example.com"
        )
        assert identity.status_code == 201
        identity_data = identity.json()["data"]
        headers = {"authorization": f"Bearer {identity_data['access_token']}"}
        workspace_id = identity_data["workspace"]["id"]
        content = b"temporary-object-that-must-be-deleted"
        initialized = await client.post(
            "/api/v1/media/uploads",
            headers=headers,
            json={
                "workspace_id": workspace_id,
                "kind": "image",
                "filename": "expired-contract.png",
                "size_bytes": len(content),
                "mime_type": "image/png",
                "sha256": hashlib.sha256(content).hexdigest(),
                "idempotency_key": "media-expiration-contract",
            },
        )
        assert initialized.status_code == 201
        initialization = initialized.json()["data"]
        upload_id = UUID(initialization["upload_session"]["id"])
        upload_url = cast(str, initialization["upload"]["url"])
        path = unquote(urlparse(upload_url).path)
        prefix = f"/{test_settings.minio_bucket}/"
        assert path.startswith(prefix)
        object_key = path.removeprefix(prefix)
        async with httpx.AsyncClient() as external:
            uploaded = await external.put(
                upload_url,
                content=content,
                headers=initialization["upload"]["headers"],
            )
        assert uploaded.status_code == 200
        await _assert_object_metadata(
            storage,
            object_key,
            size_bytes=len(content),
            content_type="image/png",
        )

        due_at = datetime.now(UTC) - timedelta(seconds=1)
        async with session_factory() as session:
            async with session.begin():
                upload = await session.get(UploadSession, upload_id, with_for_update=True)
                schedule = await session.scalar(
                    select(Schedule).where(
                        Schedule.workspace_id == UUID(workspace_id),
                        Schedule.handler_name == "expire_upload_session",
                    )
                )
                assert upload is not None and schedule is not None
                upload.expires_at = due_at
                schedule.next_fire_at = due_at

        assert (
            await dispatch_due_schedules(
                session_factory,
                dispatcher_id="media-stack-contract",
                now=datetime.now(UTC),
                batch_size=10,
                lease_duration=timedelta(seconds=30),
            )
            == 1
        )
        async with session_factory() as session:
            fire = await session.scalar(select(ScheduleFire))
            task = await session.scalar(select(Task).where(Task.task_type == "upload_expiration"))
            assert fire is not None and task is not None
            event = await session.scalar(
                select(OutboxEvent).where(OutboxEvent.aggregate_id == task.id)
            )
            assert event is not None
            envelope = envelope_from_event(event)

        await publisher.publish(envelope, "media.upload.expire")
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
        with pytest.raises(StorageObjectNotFound):
            await storage.stat(object_key)

        async with session_factory() as session:
            upload = await session.get(UploadSession, upload_id)
            task = await session.get(Task, task.id)
            delivery = await session.scalar(select(InboxDelivery))
            assert upload is not None and upload.status == "expired"
            assert upload.error_code == "upload_expired"
            assert task is not None and task.status == "succeeded"
            assert delivery is not None and delivery.status == "completed"

        await publisher.publish(envelope, "media.upload.expire")
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


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_MEDIA_STACK_CONTRACT") != "1",
    reason="set LANVERSE_RUN_MEDIA_STACK_CONTRACT=1 with isolated RabbitMQ and MinIO",
)
@pytest.mark.asyncio
async def test_workspace_cleanup_removes_only_unversioned_bytes_in_the_real_media_stack(
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
    probe = FfprobeMediaProbe(timeout_seconds=test_settings.media_probe_timeout_seconds)
    publisher = RabbitMQPublisher(rabbitmq_url)
    observer = await aio_pika.connect_robust(rabbitmq_url, timeout=3)
    message: aio_pika.abc.AbstractIncomingMessage | None = None
    temporary_key: str | None = None
    versioned_key: str | None = None
    try:
        await storage.ensure_bucket()
        await publisher.connect()
        channel = await observer.channel()
        queue = await channel.declare_queue(MEDIA_QUEUE, durable=True)
        assert queue.declaration_result.message_count == 0

        identity = await register_identity_response(
            client, email="media-cleanup-contract@example.com"
        )
        assert identity.status_code == 201
        identity_data = identity.json()["data"]
        headers = {"authorization": f"Bearer {identity_data['access_token']}"}
        workspace_id = UUID(identity_data["workspace"]["id"])

        async def initialize_and_put(*, content: bytes, idempotency_key: str) -> tuple[UUID, str]:
            initialized = await client.post(
                "/api/v1/media/uploads",
                headers=headers,
                json={
                    "workspace_id": str(workspace_id),
                    "kind": "image",
                    "filename": f"{idempotency_key}.png",
                    "size_bytes": len(content),
                    "mime_type": "image/png",
                    "sha256": hashlib.sha256(content).hexdigest(),
                    "idempotency_key": idempotency_key,
                },
            )
            assert initialized.status_code == 201
            data = initialized.json()["data"]
            upload_url = cast(str, data["upload"]["url"])
            path = unquote(urlparse(upload_url).path)
            prefix = f"/{test_settings.minio_bucket}/"
            assert path.startswith(prefix)
            object_key = path.removeprefix(prefix)
            async with httpx.AsyncClient() as external:
                uploaded = await external.put(
                    upload_url,
                    content=content,
                    headers=data["upload"]["headers"],
                )
            assert uploaded.status_code == 200
            return UUID(data["upload_session"]["id"]), object_key

        versioned_upload_id, versioned_key = await initialize_and_put(
            content=ONE_PIXEL_PNG,
            idempotency_key="cleanup-versioned",
        )
        completed = await client.post(
            f"/api/v1/media/uploads/{versioned_upload_id}/complete",
            headers=headers,
            json={},
        )
        assert completed.status_code == 200
        version_id = UUID(completed.json()["data"]["version"]["id"])

        temporary_content = b"workspace-cleanup-temporary-object"
        temporary_upload_id, temporary_key = await initialize_and_put(
            content=temporary_content,
            idempotency_key="cleanup-temporary",
        )
        due_at = datetime.now(UTC) - timedelta(seconds=1)
        async with session_factory() as session:
            async with session.begin():
                temporary_upload = await session.get(
                    UploadSession, temporary_upload_id, with_for_update=True
                )
                cleanup_schedule = await session.scalar(
                    select(Schedule).where(
                        Schedule.workspace_id == workspace_id,
                        Schedule.handler_name == "cleanup_expired_uploads",
                    )
                )
                assert temporary_upload is not None and cleanup_schedule is not None
                temporary_upload.expires_at = due_at
                cleanup_schedule.next_fire_at = due_at

        assert (
            await dispatch_due_schedules(
                session_factory,
                dispatcher_id="media-cleanup-contract",
                now=datetime.now(UTC),
                batch_size=10,
                lease_duration=timedelta(seconds=30),
            )
            == 1
        )
        async with session_factory() as session:
            task = await session.scalar(select(Task).where(Task.task_type == "upload_cleanup"))
            assert task is not None
            event = await session.scalar(
                select(OutboxEvent).where(OutboxEvent.aggregate_id == task.id)
            )
            assert event is not None
            envelope = envelope_from_event(event)

        await publisher.publish(envelope, "media.upload.cleanup")
        message = await queue.get(timeout=3, fail=False)
        assert message is not None
        assert (
            await process_incoming_message(
                message,
                session_factory,
                storage=storage,
                probe=probe,
                cleanup_batch_size=test_settings.media_cleanup_batch_size,
            )
            == "completed"
        )
        with pytest.raises(StorageObjectNotFound):
            await storage.stat(temporary_key)
        await _assert_object_metadata(
            storage,
            versioned_key,
            size_bytes=len(ONE_PIXEL_PNG),
            content_type="image/png",
        )

        async with session_factory() as session:
            temporary_upload = await session.get(UploadSession, temporary_upload_id)
            versioned_upload = await session.get(UploadSession, versioned_upload_id)
            version = await session.get(MediaVersion, version_id)
            active_location = await session.scalar(
                select(MediaLocation).where(
                    MediaLocation.media_version_id == version_id,
                    MediaLocation.status == "active",
                )
            )
            assert temporary_upload is not None and temporary_upload.status == "expired"
            assert versioned_upload is not None and versioned_upload.status == "completed"
            assert (
                version is not None and version.sha256 == hashlib.sha256(ONE_PIXEL_PNG).hexdigest()
            )
            assert active_location is not None
            assert active_location.object_key == versioned_key
        await channel.close()
    finally:
        if message is not None and not message.processed:
            await message.nack(requeue=True)
        if temporary_key is not None:
            await storage.delete(temporary_key)
        if versioned_key is not None:
            await storage.delete(versioned_key)
        await publisher.close()
        await observer.close()


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_MEDIA_STACK_CONTRACT") != "1",
    reason="set LANVERSE_RUN_MEDIA_STACK_CONTRACT=1 with isolated RabbitMQ and MinIO",
)
@pytest.mark.asyncio
async def test_media_location_migrates_rolls_back_and_retires_in_the_real_stack(
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
    probe = FfprobeMediaProbe(timeout_seconds=test_settings.media_probe_timeout_seconds)
    publisher = RabbitMQPublisher(rabbitmq_url)
    observer = await aio_pika.connect_robust(rabbitmq_url, timeout=3)
    messages: list[aio_pika.abc.AbstractIncomingMessage] = []
    source_key: str | None = None
    migrated_key: str | None = None
    try:
        await storage.ensure_bucket()
        await publisher.connect()
        channel = await observer.channel()
        queue = await channel.declare_queue(MEDIA_QUEUE, durable=True)
        assert queue.declaration_result.message_count == 0

        identity = await register_identity_response(
            client, email="media-location-contract@example.com"
        )
        assert identity.status_code == 201
        identity_data = identity.json()["data"]
        headers = {"authorization": f"Bearer {identity_data['access_token']}"}
        workspace_id = UUID(identity_data["workspace"]["id"])
        content_hash = hashlib.sha256(ONE_PIXEL_PNG).hexdigest()
        initialized = await client.post(
            "/api/v1/media/uploads",
            headers=headers,
            json={
                "workspace_id": str(workspace_id),
                "kind": "image",
                "filename": "location-contract.png",
                "size_bytes": len(ONE_PIXEL_PNG),
                "mime_type": "image/png",
                "sha256": content_hash,
                "idempotency_key": "media-location-contract-upload",
            },
        )
        assert initialized.status_code == 201
        initialization = initialized.json()["data"]
        upload_url = cast(str, initialization["upload"]["url"])
        path = unquote(urlparse(upload_url).path)
        prefix = f"/{test_settings.minio_bucket}/"
        assert path.startswith(prefix)
        source_key = path.removeprefix(prefix)
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
        version_id = UUID(completed.json()["data"]["version"]["id"])

        migration = await client.post(
            f"/api/v1/media/{version_id}/location-migrations",
            headers=headers,
            json={"idempotency_key": "media-location-contract-migrate"},
        )
        assert migration.status_code == 202
        migration_task_id = UUID(migration.json()["data"]["id"])
        async with session_factory() as session:
            migration_event = await session.scalar(
                select(OutboxEvent).where(OutboxEvent.aggregate_id == migration_task_id)
            )
            assert migration_event is not None
            migration_envelope = envelope_from_event(migration_event)
        await publisher.publish(migration_envelope, "media.location.migrate")
        migration_message = await queue.get(timeout=3, fail=False)
        assert migration_message is not None
        messages.append(migration_message)
        assert (
            await process_incoming_message(
                migration_message,
                session_factory,
                storage=storage,
                probe=probe,
                storage_profile="default",
                storage_bucket=test_settings.minio_bucket,
                location_rollback_seconds=60,
            )
            == "completed"
        )

        async with session_factory() as session:
            source = await session.scalar(
                select(MediaLocation).where(
                    MediaLocation.media_version_id == version_id,
                    MediaLocation.object_key == source_key,
                )
            )
            migrated = await session.scalar(
                select(MediaLocation).where(
                    MediaLocation.media_version_id == version_id,
                    MediaLocation.status == "active",
                )
            )
            version = await session.get(MediaVersion, version_id)
            assert source is not None and source.status == "retiring"
            assert migrated is not None and migrated.id != source.id
            assert version is not None and version.sha256 == content_hash
            migrated_key = migrated.object_key
            source_location_id = source.id
            migrated_location_id = migrated.id
        await _assert_object_metadata(
            storage,
            source_key,
            size_bytes=len(ONE_PIXEL_PNG),
            content_type="image/png",
        )
        await _assert_object_metadata(
            storage,
            migrated_key,
            size_bytes=len(ONE_PIXEL_PNG),
            content_type="image/png",
        )

        safe_locations = await client.get(f"/api/v1/media/{version_id}/locations", headers=headers)
        assert safe_locations.status_code == 200
        safe_payload = safe_locations.json()["data"]
        assert {item["status"] for item in safe_payload["items"]} == {
            "active",
            "retiring",
        }
        assert "bucket" not in str(safe_payload)
        assert "object_key" not in str(safe_payload)

        rollback = await client.post(
            f"/api/v1/media/{version_id}/location-rollbacks",
            headers=headers,
            json={
                "target_location_id": str(source_location_id),
                "idempotency_key": "media-location-contract-rollback",
            },
        )
        assert rollback.status_code == 202
        rollback_task_id = UUID(rollback.json()["data"]["id"])
        async with session_factory() as session:
            rollback_event = await session.scalar(
                select(OutboxEvent).where(OutboxEvent.aggregate_id == rollback_task_id)
            )
            assert rollback_event is not None
            rollback_envelope = envelope_from_event(rollback_event)
        await publisher.publish(rollback_envelope, "media.location.migrate")
        rollback_message = await queue.get(timeout=3, fail=False)
        assert rollback_message is not None
        messages.append(rollback_message)
        assert (
            await process_incoming_message(
                rollback_message,
                session_factory,
                storage=storage,
                probe=probe,
                storage_profile="default",
                storage_bucket=test_settings.minio_bucket,
                location_rollback_seconds=60,
            )
            == "completed"
        )

        due_at = datetime.now(UTC) - timedelta(seconds=1)
        async with session_factory() as session:
            async with session.begin():
                source = await session.get(MediaLocation, source_location_id, with_for_update=True)
                migrated = await session.get(
                    MediaLocation, migrated_location_id, with_for_update=True
                )
                retirement_schedule = await session.scalar(
                    select(Schedule).where(
                        Schedule.schedule_key == f"media.location.retire:{migrated_location_id}"
                    )
                )
                assert source is not None and source.status == "active"
                assert migrated is not None and migrated.status == "retiring"
                assert retirement_schedule is not None
                migrated.retire_after = due_at
                retirement_schedule.next_fire_at = due_at

        assert (
            await dispatch_due_schedules(
                session_factory,
                dispatcher_id="media-location-contract",
                now=datetime.now(UTC),
                batch_size=10,
                lease_duration=timedelta(seconds=30),
            )
            == 1
        )
        async with session_factory() as session:
            retirement_task = await session.scalar(
                select(Task).where(Task.task_type == "media_location_retirement")
            )
            assert retirement_task is not None
            retirement_event = await session.scalar(
                select(OutboxEvent).where(OutboxEvent.aggregate_id == retirement_task.id)
            )
            assert retirement_event is not None
            retirement_envelope = envelope_from_event(retirement_event)
        await publisher.publish(retirement_envelope, "media.location.retire")
        retirement_message = await queue.get(timeout=3, fail=False)
        assert retirement_message is not None
        messages.append(retirement_message)
        assert (
            await process_incoming_message(
                retirement_message,
                session_factory,
                storage=storage,
                probe=probe,
                storage_profile="default",
                storage_bucket=test_settings.minio_bucket,
                location_rollback_seconds=60,
            )
            == "completed"
        )
        await _assert_object_metadata(
            storage,
            source_key,
            size_bytes=len(ONE_PIXEL_PNG),
            content_type="image/png",
        )
        with pytest.raises(StorageObjectNotFound):
            await storage.stat(migrated_key)
        async with session_factory() as session:
            version = await session.get(MediaVersion, version_id)
            source = await session.get(MediaLocation, source_location_id)
            migrated = await session.get(MediaLocation, migrated_location_id)
            assert version is not None
            assert version.id == version_id
            assert version.sha256 == content_hash
            assert version.size_bytes == len(ONE_PIXEL_PNG)
            assert source is not None and source.status == "active"
            assert migrated is not None and migrated.status == "retired"
            assert migrated.retired_at is not None
        await channel.close()
    finally:
        for message in messages:
            if not message.processed:
                await message.nack(requeue=True)
        if source_key is not None:
            await storage.delete(source_key)
        if migrated_key is not None:
            await storage.delete(migrated_key)
        await publisher.close()
        await observer.close()
