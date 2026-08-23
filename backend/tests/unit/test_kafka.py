import pytest

from app.integrations.kafka import (
    IO_TOPIC,
    MEDIA_TOPIC,
    REGISTERED_TOPICS,
    KafkaIncomingMessage,
)


def test_kafka_registers_only_worker_boundary_topics() -> None:
    assert REGISTERED_TOPICS == frozenset({IO_TOPIC, MEDIA_TOPIC})
    assert IO_TOPIC == "lanverse.io.v1"
    assert MEDIA_TOPIC == "lanverse.media.v1"


@pytest.mark.asyncio
async def test_kafka_message_ack_commits_the_offset() -> None:
    message = KafkaIncomingMessage(body=b"{}", headers=[("traceparent", b"trace")])

    await message.ack()

    assert message.should_commit is True
    assert message.should_retry is False
    assert message.headers == {"traceparent": b"trace"}


@pytest.mark.asyncio
async def test_kafka_message_requeue_keeps_the_offset_uncommitted() -> None:
    message = KafkaIncomingMessage(body=b"{}", headers=[])

    await message.nack(requeue=True)

    assert message.should_commit is False
    assert message.should_retry is True


@pytest.mark.asyncio
async def test_kafka_message_disposition_is_final() -> None:
    message = KafkaIncomingMessage(body=b"{}", headers=[])
    await message.ack()

    with pytest.raises(RuntimeError, match="already decided"):
        await message.nack(requeue=True)
