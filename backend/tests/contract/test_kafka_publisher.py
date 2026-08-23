import asyncio
import os
from datetime import UTC, datetime
from typing import Any

import pytest
from aiokafka import AIOKafkaConsumer  # pyright: ignore[reportMissingTypeStubs]
from uuid6 import uuid7

from app.integrations.kafka import IO_TOPIC, KafkaPublisher
from app.modules.messaging import MessageEnvelope
from tests.support.external_contracts import kafka_contract_bootstrap_servers


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_KAFKA_CONTRACT") != "1",
    reason="set LANVERSE_RUN_KAFKA_CONTRACT=1 with the local Kafka broker",
)
@pytest.mark.asyncio
async def test_kafka_publishes_confirmed_keyed_versioned_envelope() -> None:
    bootstrap_servers = kafka_contract_bootstrap_servers()
    event_id = uuid7()
    task_id = uuid7()
    envelope = MessageEnvelope(
        event_id=event_id,
        event_type="script_extraction.requested",
        schema_version=1,
        aggregate_id=task_id,
        workspace_id=uuid7(),
        occurred_at=datetime.now(UTC),
        trace_id="kafka-contract-trace",
        traceparent="00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
        causation_event_id=None,
        payload={"task_id": str(task_id)},
    )
    publisher = KafkaPublisher(bootstrap_servers)
    consumer: Any = AIOKafkaConsumer(
        IO_TOPIC,
        bootstrap_servers=bootstrap_servers,
        group_id=f"lanverse-contract-publisher-{uuid7()}",
        enable_auto_commit=False,
        auto_offset_reset="latest",
    )
    await consumer.start()
    try:
        await publisher.connect()
        await publisher.publish(envelope, IO_TOPIC)

        record = await asyncio.wait_for(consumer.getone(), timeout=5)
        headers = dict(record.headers)
        assert record.topic == IO_TOPIC
        assert record.key == str(task_id).encode("ascii")
        assert headers["event_id"] == str(event_id).encode("ascii")
        assert headers["event_type"] == envelope.event_type.encode("utf-8")
        assert headers["trace_id"] == envelope.trace_id.encode("ascii")
        assert envelope.traceparent is not None
        assert headers["traceparent"] == envelope.traceparent.encode("ascii")
        assert MessageEnvelope.model_validate_json(record.value) == envelope
        await consumer.commit()
    finally:
        await publisher.close()
        await consumer.stop()
