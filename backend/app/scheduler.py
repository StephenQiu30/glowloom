import asyncio
import os
import socket
from datetime import UTC, datetime, timedelta

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.core.config import Settings, get_settings
from app.core.database import session_factory
from app.integrations.rabbitmq import RabbitMQPublisher
from app.modules.messaging import (
    MessagePublisher,
    claim_outbox_events,
    envelope_from_event,
    mark_outbox_published,
    release_outbox_for_retry,
)


async def publish_outbox_batch(
    factory: async_sessionmaker[AsyncSession],
    publisher: MessagePublisher,
    *,
    publisher_id: str,
    batch_size: int,
    claim_timeout: timedelta,
) -> int:
    claimed_at = datetime.now(UTC)
    async with factory() as session:
        async with session.begin():
            events = await claim_outbox_events(
                session,
                publisher_id=publisher_id,
                now=claimed_at,
                batch_size=batch_size,
                claim_timeout=claim_timeout,
            )

    published = 0
    for event in events:
        try:
            await publisher.publish(envelope_from_event(event), event.routing_key)
        except Exception as error:
            async with factory() as session:
                async with session.begin():
                    await release_outbox_for_retry(
                        session,
                        event.id,
                        publisher_id=publisher_id,
                        now=datetime.now(UTC),
                        error=error,
                    )
        else:
            async with factory() as session:
                async with session.begin():
                    await mark_outbox_published(
                        session,
                        event.id,
                        publisher_id=publisher_id,
                        now=datetime.now(UTC),
                    )
            published += 1
    return published


async def run_scheduler(settings: Settings) -> None:
    publisher = RabbitMQPublisher(settings.rabbitmq_url)
    publisher_id = f"{socket.gethostname()}:{os.getpid()}"
    await publisher.connect()
    try:
        while True:
            published = await publish_outbox_batch(
                session_factory,
                publisher,
                publisher_id=publisher_id,
                batch_size=settings.outbox_batch_size,
                claim_timeout=timedelta(seconds=settings.outbox_claim_seconds),
            )
            if published == 0:
                await asyncio.sleep(settings.outbox_poll_seconds)
    finally:
        await publisher.close()


def main() -> None:
    asyncio.run(run_scheduler(get_settings()))


if __name__ == "__main__":
    main()
