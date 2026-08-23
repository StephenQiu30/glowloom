import asyncio
import contextlib
import os
from datetime import UTC, datetime

import pytest
from uuid6 import uuid7

from app.integrations.kafka import IO_TOPIC, KafkaIncomingMessage, KafkaPublisher, consume_topic
from app.modules.messaging import MessageEnvelope
from tests.support.external_contracts import kafka_contract_bootstrap_servers


def _envelope(trace_id: str) -> MessageEnvelope:
    task_id = uuid7()
    return MessageEnvelope(
        event_id=uuid7(),
        event_type="script_extraction.requested",
        schema_version=1,
        aggregate_id=task_id,
        workspace_id=uuid7(),
        occurred_at=datetime.now(UTC),
        trace_id=trace_id,
        traceparent="00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
        causation_event_id=None,
        payload={"task_id": str(task_id)},
    )


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_KAFKA_CONTRACT") != "1",
    reason="set LANVERSE_RUN_KAFKA_CONTRACT=1 with the local Kafka broker",
)
@pytest.mark.asyncio
async def test_kafka_worker_retries_without_commit_then_commits_success() -> None:
    bootstrap_servers = kafka_contract_bootstrap_servers()
    publisher = KafkaPublisher(bootstrap_servers)
    completed = asyncio.Event()
    attempts = 0
    target_trace_id = f"kafka-retry-contract-{uuid7()}"

    async def handler(message: KafkaIncomingMessage) -> None:
        nonlocal attempts
        envelope = MessageEnvelope.model_validate_json(message.body)
        if envelope.trace_id != target_trace_id:
            await message.ack()
            return
        attempts += 1
        if attempts == 1:
            await message.nack(requeue=True)
            return
        await message.ack()
        completed.set()

    consumer_task = asyncio.create_task(
        consume_topic(
            bootstrap_servers,
            topic=IO_TOPIC,
            group_id=f"lanverse-contract-retry-{uuid7()}",
            handler=handler,
            retry_backoff_seconds=0.01,
        )
    )
    try:
        await publisher.connect()
        await asyncio.sleep(0.2)
        await publisher.publish(_envelope(target_trace_id), IO_TOPIC)
        await asyncio.wait_for(completed.wait(), timeout=5)
        await asyncio.sleep(0.1)
        assert attempts == 2
    finally:
        consumer_task.cancel()
        with contextlib.suppress(asyncio.CancelledError):
            await consumer_task
        await publisher.close()
