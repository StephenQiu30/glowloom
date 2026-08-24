import asyncio

import pytest

from app.integrations.kafka import (
    IO_TOPIC,
    MEDIA_TOPIC,
    REGISTERED_TOPICS,
    KafkaIncomingMessage,
    _run_handler_while_polling,  # pyright: ignore[reportPrivateUsage]
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


@pytest.mark.asyncio
async def test_long_handler_keeps_polling_without_a_wall_clock_deadline() -> None:
    partition = object()

    class FakeConsumer:
        def __init__(self) -> None:
            self.paused_partitions: tuple[object, ...] = ()
            self.resumed_partitions: tuple[object, ...] = ()
            self.polls = 0
            self.poll_partitions: list[tuple[object, ...]] = []
            self.polled_twice = asyncio.Event()

        def assignment(self) -> set[object]:
            return {partition}

        def paused(self) -> set[object]:
            return set(self.paused_partitions)

        def pause(self, *partitions: object) -> None:
            self.paused_partitions = partitions

        def resume(self, *partitions: object) -> None:
            self.resumed_partitions = partitions

        async def getmany(
            self,
            *partitions: object,
            **_: object,
        ) -> dict[object, object]:
            self.polls += 1
            self.poll_partitions.append(partitions)
            if self.polls >= 2:
                self.polled_twice.set()
            return {}

    consumer = FakeConsumer()
    release = asyncio.Event()
    message = KafkaIncomingMessage(body=b"{}", headers=[])

    async def handler(incoming: KafkaIncomingMessage) -> str:
        await release.wait()
        await incoming.ack()
        return "completed"

    task = asyncio.create_task(
        _run_handler_while_polling(
            consumer,
            message,
            handler,
            keepalive_poll_seconds=0.001,
        )
    )
    await consumer.polled_twice.wait()
    release.set()

    assert await task == "completed"
    assert consumer.paused_partitions == (partition,)
    assert consumer.resumed_partitions == (partition,)
    assert consumer.polls >= 2
    assert consumer.poll_partitions == [(partition,)] * consumer.polls
    assert message.should_commit is True
