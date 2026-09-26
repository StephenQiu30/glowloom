"""Run mock provider Activities against the configured Temporal and Redis services."""

import asyncio
from contextlib import suppress
from pathlib import Path

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict
from redis.asyncio import Redis
from temporalio.client import Client

from app.harness.mock_model import MockStructuredClient
from app.harness.router import ModelRouter, Price
from app.harness.skills import SkillRegistry
from app.providers.mock import MockProvider
from app.worker.provider_activities import create_mock_worker
from app.worker.skill_activities import create_skill_worker


class WorkerSettings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="LV_", extra="ignore")

    temporal_addr: str = Field(min_length=1)
    temporal_namespace: str = Field(min_length=1)
    redis_url: str = Field(min_length=1)


async def run(settings: WorkerSettings, registry: SkillRegistry) -> None:
    router = ModelRouter(MockStructuredClient(), {"mock.structured": Price(0, 0)})
    client = await Client.connect(settings.temporal_addr, namespace=settings.temporal_namespace)
    async with Redis.from_url(settings.redis_url, decode_responses=True) as store:
        await store.ping()
        async with asyncio.TaskGroup() as workers:
            workers.create_task(create_mock_worker(client, MockProvider(store)).run())
            workers.create_task(create_skill_worker(client, registry, router).run())


def main() -> None:
    settings = WorkerSettings()
    registry = SkillRegistry.load(Path(__file__).resolve().parent.parent / "skills")
    with suppress(KeyboardInterrupt):
        asyncio.run(run(settings, registry))


if __name__ == "__main__":
    main()
