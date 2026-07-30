import asyncio
from typing import Literal, Protocol

import aio_pika
from pydantic import ValidationError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings, get_settings
from app.core.database import session_factory
from app.integrations.ffprobe import FfprobeMediaProbe
from app.integrations.minio import MinioObjectStorage
from app.integrations.rabbitmq import declare_task_topology
from app.model_registry import register_implemented_models
from app.modules.media import MediaProbePort
from app.modules.media.consumer import consume_media_probe
from app.modules.media.storage import ObjectStoragePort
from app.modules.messaging import MessageEnvelope

MEDIA_WORKER_MAX_IN_FLIGHT = 2
MAX_MESSAGE_BYTES = 64 * 1024
WorkerResult = Literal["completed", "duplicate", "rejected", "requeued"]


class IncomingMessage(Protocol):
    body: bytes

    async def ack(self) -> None: ...

    async def nack(self, *, requeue: bool) -> None: ...


async def process_incoming_message(
    message: IncomingMessage,
    factory: async_sessionmaker[AsyncSession],
    *,
    storage: ObjectStoragePort,
    probe: MediaProbePort,
) -> WorkerResult:
    if len(message.body) > MAX_MESSAGE_BYTES:
        await message.ack()
        return "rejected"
    try:
        envelope = MessageEnvelope.model_validate_json(message.body)
    except ValidationError:
        await message.ack()
        return "rejected"
    try:
        async with factory() as session:
            async with session.begin():
                result = await consume_media_probe(
                    session,
                    envelope,
                    storage=storage,
                    probe=probe,
                )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"
    await message.ack()
    return result


async def run_media_worker(settings: Settings) -> None:
    register_implemented_models()
    storage = MinioObjectStorage(
        settings.minio_endpoint,
        settings.minio_access_key,
        settings.minio_secret_key,
        settings.minio_bucket,
        secure=settings.minio_secure,
        thread_limit=settings.storage_thread_limit,
    )
    probe = FfprobeMediaProbe(timeout_seconds=settings.media_probe_timeout_seconds)
    connection = await aio_pika.connect_robust(settings.rabbitmq_url, timeout=3)
    try:
        channel = await connection.channel()
        await channel.set_qos(prefetch_count=MEDIA_WORKER_MAX_IN_FLIGHT)
        _, _, media_queue = await declare_task_topology(channel)

        async def on_message(message: aio_pika.abc.AbstractIncomingMessage) -> None:
            await process_incoming_message(
                message,
                session_factory,
                storage=storage,
                probe=probe,
            )

        await media_queue.consume(on_message, no_ack=False)
        await asyncio.Future()
    finally:
        await connection.close()


def main() -> None:
    asyncio.run(run_media_worker(get_settings()))


if __name__ == "__main__":
    main()
