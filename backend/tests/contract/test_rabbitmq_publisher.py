import os
from datetime import UTC, datetime

import aio_pika
import pytest
from uuid6 import uuid7

from app.integrations.rabbitmq import IO_QUEUE, RabbitMQPublisher
from app.modules.messaging import MessageEnvelope
from tests.support.external_contracts import rabbitmq_contract_url


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_RABBITMQ_CONTRACT") != "1",
    reason="set LANVERSE_RUN_RABBITMQ_CONTRACT=1 with an isolated RabbitMQ vhost",
)
@pytest.mark.asyncio
async def test_rabbitmq_publishes_confirmed_persistent_versioned_envelope() -> None:
    rabbitmq_url = rabbitmq_contract_url()
    event_id = uuid7()
    task_id = uuid7()
    envelope = MessageEnvelope(
        event_id=event_id,
        event_type="script_extraction.requested",
        schema_version=1,
        aggregate_id=task_id,
        workspace_id=uuid7(),
        occurred_at=datetime.now(UTC),
        trace_id="rabbitmq-contract-trace",
        causation_event_id=None,
        payload={"task_id": str(task_id)},
    )
    publisher = RabbitMQPublisher(rabbitmq_url)
    observer = await aio_pika.connect_robust(rabbitmq_url, timeout=3)
    try:
        await publisher.connect()
        channel = await observer.channel()
        queue = await channel.declare_queue(IO_QUEUE, durable=True)
        await publisher.publish(envelope, "io.script.extract")

        held: list[aio_pika.abc.AbstractIncomingMessage] = []
        matched: aio_pika.abc.AbstractIncomingMessage | None = None
        for _ in range(50):
            message = await queue.get(timeout=3, fail=False)
            if message is None:
                break
            if message.message_id == str(event_id):
                matched = message
                break
            held.append(message)
        for message in held:
            await message.nack(requeue=True)

        assert matched is not None
        assert matched.delivery_mode == aio_pika.DeliveryMode.PERSISTENT
        assert matched.content_type == "application/json"
        assert matched.type == envelope.event_type
        assert matched.correlation_id == envelope.trace_id
        assert MessageEnvelope.model_validate_json(matched.body) == envelope
        await matched.ack()
        await channel.close()
    finally:
        await publisher.close()
        await observer.close()
