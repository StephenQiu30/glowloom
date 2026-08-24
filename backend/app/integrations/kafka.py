import asyncio
from collections.abc import Awaitable, Callable, Iterable
from typing import Any, cast

from aiokafka import AIOKafkaConsumer, AIOKafkaProducer  # pyright: ignore[reportMissingTypeStubs]
from aiokafka.structs import (  # pyright: ignore[reportMissingTypeStubs]
    OffsetAndMetadata,
    TopicPartition,
)

from app.modules.messaging import MessageEnvelope
from app.modules.messaging.topics import IO_TOPIC, MEDIA_TOPIC, REGISTERED_TOPICS

MAX_MESSAGE_BYTES = 64 * 1024
KEEPALIVE_POLL_SECONDS = 1.0
MessageHandler = Callable[["KafkaIncomingMessage"], Awaitable[object]]


class KafkaIncomingMessage:
    def __init__(
        self,
        *,
        body: bytes,
        headers: Iterable[tuple[str, bytes | None]],
    ) -> None:
        self.body = body
        self.headers = {key: value for key, value in headers if value is not None}
        self._disposition: str | None = None

    @property
    def should_commit(self) -> bool:
        return self._disposition == "commit"

    @property
    def should_retry(self) -> bool:
        return self._disposition == "retry"

    @property
    def processed(self) -> bool:
        return self._disposition is not None

    async def ack(self) -> None:
        self._decide("commit")

    async def nack(self, *, requeue: bool) -> None:
        self._decide("retry" if requeue else "commit")

    def _decide(self, disposition: str) -> None:
        if self._disposition is not None:
            raise RuntimeError("Kafka message disposition is already decided")
        self._disposition = disposition


class KafkaPublisher:
    def __init__(self, bootstrap_servers: str) -> None:
        self._bootstrap_servers = bootstrap_servers
        self._producer: Any | None = None

    async def connect(self) -> None:
        if self._producer is not None:
            return
        producer = AIOKafkaProducer(
            bootstrap_servers=self._bootstrap_servers,
            client_id="lanverse-outbox-v1",
            acks="all",
            enable_idempotence=True,
            max_request_size=MAX_MESSAGE_BYTES,
        )
        try:
            await producer.start()
        except Exception:
            await producer.stop()
            raise
        self._producer = producer

    async def publish(self, envelope: MessageEnvelope, topic: str) -> None:
        if topic not in REGISTERED_TOPICS:
            raise ValueError("Kafka topic is not registered")
        if self._producer is None:
            raise RuntimeError("Kafka publisher is not connected")
        headers = [
            ("event_id", str(envelope.event_id).encode("ascii")),
            ("event_type", envelope.event_type.encode("utf-8")),
            ("trace_id", envelope.trace_id.encode("ascii")),
        ]
        if envelope.traceparent is not None:
            headers.append(("traceparent", envelope.traceparent.encode("ascii")))
        await self._producer.send_and_wait(
            topic,
            envelope.model_dump_json().encode("utf-8"),
            key=str(envelope.aggregate_id).encode("ascii"),
            headers=headers,
        )

    async def close(self) -> None:
        if self._producer is not None:
            await self._producer.stop()
            self._producer = None


async def consume_topic(
    bootstrap_servers: str,
    *,
    topic: str,
    group_id: str,
    handler: MessageHandler,
    retry_backoff_seconds: float = 1.0,
) -> None:
    if topic not in REGISTERED_TOPICS:
        raise ValueError("Kafka topic is not registered")
    consumer: Any = AIOKafkaConsumer(
        topic,
        bootstrap_servers=bootstrap_servers,
        client_id=f"{group_id}-consumer",
        group_id=group_id,
        enable_auto_commit=False,
        auto_offset_reset="earliest",
        max_partition_fetch_bytes=MAX_MESSAGE_BYTES,
    )
    try:
        await consumer.start()
    except Exception:
        await consumer.stop()
        raise
    try:
        async for record in consumer:
            message = KafkaIncomingMessage(
                body=cast(bytes | None, record.value) or b"",
                headers=cast(list[tuple[str, bytes | None]], record.headers),
            )
            await _run_handler_while_polling(consumer, message, handler)
            partition = TopicPartition(record.topic, record.partition)
            if message.should_retry:
                consumer.seek(partition, record.offset)
                await asyncio.sleep(retry_backoff_seconds)
                continue
            if not message.should_commit:
                raise RuntimeError("Kafka handler returned without deciding the offset")
            await consumer.commit(
                {
                    partition: OffsetAndMetadata(
                        record.offset + 1,
                        "",
                    )
                }
            )
    finally:
        await consumer.stop()


async def _run_handler_while_polling(
    consumer: Any,
    message: KafkaIncomingMessage,
    handler: MessageHandler,
    *,
    keepalive_poll_seconds: float = KEEPALIVE_POLL_SECONDS,
) -> object:
    """Keep the consumer polling while one durable handler owns its message."""

    assigned = tuple(consumer.assignment())
    already_paused = set(consumer.paused())
    paused_here = tuple(partition for partition in assigned if partition not in already_paused)
    if paused_here:
        consumer.pause(*paused_here)
    handler_task: asyncio.Task[object] = asyncio.create_task(
        _invoke_handler(handler, message)
    )
    poll_task: asyncio.Task[object] | None = None
    try:
        while not handler_task.done():
            poll_task = asyncio.create_task(
                _poll_paused_partitions(
                    consumer,
                    assigned,
                    delay_seconds=keepalive_poll_seconds,
                )
            )
            done, _ = await asyncio.wait(
                {handler_task, poll_task},
                return_when=asyncio.FIRST_COMPLETED,
            )
            if poll_task in done:
                polled = await poll_task
                poll_task = None
                if polled:
                    raise RuntimeError("Paused Kafka partitions unexpectedly returned records")
            if handler_task in done:
                if poll_task is not None:
                    poll_task.cancel()
                    try:
                        await poll_task
                    except BaseException:
                        pass
                    poll_task = None
                break
        return await handler_task
    finally:
        if poll_task is not None and not poll_task.done():
            poll_task.cancel()
            try:
                await poll_task
            except BaseException:
                pass
        if not handler_task.done():
            handler_task.cancel()
            try:
                await handler_task
            except BaseException:
                pass
        current_assignment = set(consumer.assignment())
        resumable = tuple(
            partition for partition in paused_here if partition in current_assignment
        )
        if resumable:
            consumer.resume(*resumable)


async def _invoke_handler(
    handler: MessageHandler,
    message: KafkaIncomingMessage,
) -> object:
    return await handler(message)


async def _poll_paused_partitions(
    consumer: Any,
    partitions: tuple[Any, ...],
    *,
    delay_seconds: float,
) -> object:
    if not partitions:
        raise RuntimeError("Kafka handler lost its original partition assignment")
    await asyncio.sleep(delay_seconds)
    # Explicit partitions prevent a rebalance from exposing newly assigned
    # records to this keepalive poll. Its result is still checked fail closed.
    return await consumer.getmany(
        *partitions,
        timeout_ms=0,
        max_records=1,
    )


async def kafka_ping(bootstrap_servers: str) -> None:
    publisher = KafkaPublisher(bootstrap_servers)
    try:
        await publisher.connect()
    finally:
        await publisher.close()


__all__ = [
    "IO_TOPIC",
    "MEDIA_TOPIC",
    "REGISTERED_TOPICS",
    "KafkaIncomingMessage",
    "KafkaPublisher",
    "consume_topic",
    "kafka_ping",
]
