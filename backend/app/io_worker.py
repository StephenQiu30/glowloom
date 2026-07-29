import asyncio
from typing import Literal, Protocol

import aio_pika
from pydantic import ValidationError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings, get_settings
from app.core.database import session_factory
from app.integrations.rabbitmq import declare_task_topology
from app.modules.messaging.consumer import (
    IO_SCRIPT_EXTRACTION_CONSUMER,
    consume_envelope,
)
from app.modules.messaging.schemas import MessageEnvelope

IO_WORKER_MAX_IN_FLIGHT = 4
MAX_MESSAGE_BYTES = 64 * 1024
WorkerResult = Literal["completed", "duplicate", "rejected", "requeued"]


class IncomingMessage(Protocol):
    body: bytes

    async def ack(self) -> None: ...

    async def nack(self, *, requeue: bool) -> None: ...


async def process_incoming_message(
    message: IncomingMessage,
    factory: async_sessionmaker[AsyncSession],
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
                result = await consume_envelope(
                    session,
                    envelope,
                    consumer_name=IO_SCRIPT_EXTRACTION_CONSUMER,
                )
    except Exception:
        await message.nack(requeue=True)
        return "requeued"

    await message.ack()
    return result


async def run_io_worker(settings: Settings) -> None:
    connection = await aio_pika.connect_robust(settings.rabbitmq_url, timeout=3)
    try:
        channel = await connection.channel()
        await channel.set_qos(prefetch_count=IO_WORKER_MAX_IN_FLIGHT)
        _, io_queue, _ = await declare_task_topology(channel)

        async def on_message(message: aio_pika.abc.AbstractIncomingMessage) -> None:
            await process_incoming_message(message, session_factory)

        await io_queue.consume(on_message, no_ack=False)
        await asyncio.Future()
    finally:
        await connection.close()


def main() -> None:
    asyncio.run(run_io_worker(get_settings()))


if __name__ == "__main__":
    main()
