"""Run mock provider Activities against the configured Temporal and Redis services."""

import asyncio
from contextlib import suppress

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict
from redis.asyncio import Redis
from temporalio.client import Client

from app.providers.mock import MockProvider
from app.worker.provider_activities import create_mock_worker


class WorkerSettings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="LV_", extra="ignore")

    temporal_addr: str = Field(min_length=1)
    temporal_namespace: str = Field(min_length=1)
    redis_url: str = Field(min_length=1)


async def run(settings: WorkerSettings) -> None:
    client = await Client.connect(settings.temporal_addr, namespace=settings.temporal_namespace)
    async with Redis.from_url(settings.redis_url, decode_responses=True) as store:
        await store.ping()
        worker = create_mock_worker(client, MockProvider(store))
        await worker.run()


def main() -> None:
    with suppress(KeyboardInterrupt):
        asyncio.run(run(WorkerSettings()))


if __name__ == "__main__":
    main()
