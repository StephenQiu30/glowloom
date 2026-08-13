import aio_pika
from aio_pika.abc import (
    AbstractChannel,
    AbstractExchange,
    AbstractQueue,
    AbstractRobustConnection,
)

from app.modules.messaging import MessageEnvelope

EXCHANGE = "lanverse.tasks.v1"
IO_QUEUE = "lanverse.io"
MEDIA_QUEUE = "lanverse.media"
ALLOWED_ROUTING_KEYS = frozenset(
    {
        "io.script.extract",
        "io.script.plan",
        "io.script.adapt",
        "io.provider.submit",
        "io.provider.query",
        "io.provider.cancel",
        "io.provider.download",
        "io.moderation.check",
        "media.probe",
        "media.transcode",
        "media.preview",
        "media.render",
        "media.label",
        "media.upload.expire",
        "media.upload.cleanup",
        "media.location.migrate",
        "media.location.retire",
        "media.storyboard.export",
    }
)


async def declare_task_topology(
    channel: AbstractChannel,
) -> tuple[AbstractExchange, AbstractQueue, AbstractQueue]:
    exchange = await channel.declare_exchange(EXCHANGE, aio_pika.ExchangeType.TOPIC, durable=True)
    io_queue = await channel.declare_queue(IO_QUEUE, durable=True)
    media_queue = await channel.declare_queue(MEDIA_QUEUE, durable=True)
    await io_queue.bind(exchange, routing_key="io.#")
    await media_queue.bind(exchange, routing_key="media.#")
    return exchange, io_queue, media_queue


class RabbitMQPublisher:
    def __init__(self, url: str) -> None:
        self._url = url
        self._connection: AbstractRobustConnection | None = None
        self._channel: AbstractChannel | None = None
        self._exchange: AbstractExchange | None = None

    async def connect(self) -> None:
        if self._connection is not None:
            return
        connection = await aio_pika.connect_robust(self._url, timeout=3)
        channel = await connection.channel(publisher_confirms=True)
        exchange, _, _ = await declare_task_topology(channel)
        self._connection = connection
        self._channel = channel
        self._exchange = exchange

    async def publish(self, envelope: MessageEnvelope, routing_key: str) -> None:
        if routing_key not in ALLOWED_ROUTING_KEYS:
            raise ValueError("routing key is not registered")
        if self._exchange is None:
            raise RuntimeError("RabbitMQ publisher is not connected")
        await self._exchange.publish(
            aio_pika.Message(
                body=envelope.model_dump_json().encode("utf-8"),
                content_type="application/json",
                content_encoding="utf-8",
                delivery_mode=aio_pika.DeliveryMode.PERSISTENT,
                message_id=str(envelope.event_id),
                type=envelope.event_type,
                correlation_id=envelope.trace_id,
                headers={"traceparent": envelope.traceparent}
                if envelope.traceparent is not None
                else None,
            ),
            routing_key=routing_key,
            mandatory=True,
        )

    async def close(self) -> None:
        if self._channel is not None and not self._channel.is_closed:
            await self._channel.close()
        if self._connection is not None and not self._connection.is_closed:
            await self._connection.close()
        self._exchange = None
        self._channel = None
        self._connection = None


async def rabbitmq_ping(url: str) -> None:
    publisher = RabbitMQPublisher(url)
    try:
        await publisher.connect()
    finally:
        await publisher.close()
