import asyncio
from typing import Any

from aiokafka import AIOKafkaConsumer  # pyright: ignore[reportMissingTypeStubs]
from aiokafka.structs import (  # pyright: ignore[reportMissingTypeStubs]
    OffsetAndMetadata,
    TopicPartition,
)
from uuid6 import uuid7

from app.integrations.kafka import KafkaIncomingMessage


class KafkaContractMessage(KafkaIncomingMessage):
    def __init__(self, consumer: Any, record: Any) -> None:
        super().__init__(body=record.value or b"", headers=record.headers)
        self._consumer = consumer
        self._partition = TopicPartition(record.topic, record.partition)
        self._offset = int(record.offset)

    async def ack(self) -> None:
        await super().ack()
        await self._consumer.commit({self._partition: OffsetAndMetadata(self._offset + 1, "")})

    async def nack(self, *, requeue: bool) -> None:
        await super().nack(requeue=requeue)
        if requeue:
            self._consumer.seek(self._partition, self._offset)
        else:
            await self._consumer.commit({self._partition: OffsetAndMetadata(self._offset + 1, "")})


class KafkaContractObserver:
    def __init__(self, bootstrap_servers: str, *, topic: str) -> None:
        self._consumer: Any = AIOKafkaConsumer(
            topic,
            bootstrap_servers=bootstrap_servers,
            group_id=f"lanverse-contract-observer-{uuid7()}",
            enable_auto_commit=False,
            auto_offset_reset="latest",
        )

    async def start(self) -> None:
        await self._consumer.start()

    async def get(
        self,
        *,
        wait_seconds: float,
        fail: bool = False,
    ) -> KafkaContractMessage | None:
        try:
            record = await asyncio.wait_for(self._consumer.getone(), timeout=wait_seconds)
        except TimeoutError:
            if fail:
                raise
            return None
        return KafkaContractMessage(self._consumer, record)

    async def close(self) -> None:
        await self._consumer.stop()
