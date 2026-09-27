"""Run mock provider Activities against the configured Temporal and Redis services."""

import asyncio
from contextlib import suppress
from pathlib import Path

import httpx
from pydantic import Field, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict
from redis.asyncio import Redis
from temporalio.client import Client

from app.harness.mock_model import MockStructuredClient
from app.harness.router import ModelRouter, Price
from app.harness.skills import SkillRegistry
from app.providers.credential_crypto import CredentialOpener
from app.providers.mock import MockProvider
from app.worker.credential_activities import CredentialTestActivities
from app.worker.provider_activities import create_mock_worker
from app.worker.skill_activities import create_skill_worker


class WorkerSettings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="LV_", env_ignore_empty=True, extra="ignore")

    temporal_addr: str = Field(min_length=1)
    temporal_namespace: str = Field(min_length=1)
    redis_url: str = Field(min_length=1)
    credential_key_id: str | None = None
    credential_private_key_ref: Path | None = None

    @model_validator(mode="after")
    def complete_credential_key(self) -> "WorkerSettings":
        if (self.credential_key_id is None) != (self.credential_private_key_ref is None):
            raise ValueError("credential key ID and private key file must be configured together")
        if (
            self.credential_private_key_ref is not None
            and not self.credential_private_key_ref.is_absolute()
        ):
            raise ValueError("credential private key file must use an absolute path")
        return self


async def run(settings: WorkerSettings, registry: SkillRegistry) -> None:
    router = ModelRouter(MockStructuredClient(), {"mock.structured": Price(0, 0)})
    client = await Client.connect(settings.temporal_addr, namespace=settings.temporal_namespace)
    async with (
        Redis.from_url(settings.redis_url, decode_responses=True) as store,
        httpx.AsyncClient(timeout=5.0, follow_redirects=False, trust_env=False) as http_client,
    ):
        await store.ping()
        opener = None
        if (
            settings.credential_private_key_ref is not None
            and settings.credential_key_id is not None
        ):
            try:
                key_bytes = settings.credential_private_key_ref.read_bytes()
            except OSError:
                raise ValueError("credential private key file is unreadable") from None
            opener = CredentialOpener(settings.credential_key_id, key_bytes)
        credential_tests = CredentialTestActivities(opener, http_client)
        async with asyncio.TaskGroup() as workers:
            workers.create_task(create_mock_worker(client, MockProvider(store)).run())
            workers.create_task(
                create_skill_worker(
                    client, registry, router, credential_tests=credential_tests
                ).run()
            )


def main() -> None:
    settings = WorkerSettings()
    registry = SkillRegistry.load(Path(__file__).resolve().parent.parent / "skills")
    with suppress(KeyboardInterrupt):
        asyncio.run(run(settings, registry))


if __name__ == "__main__":
    main()
