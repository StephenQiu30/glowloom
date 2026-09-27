"""Run mock provider Activities against the configured Temporal and Redis services."""

import asyncio
from contextlib import suppress
from pathlib import Path
from urllib.parse import urlsplit

import httpx
from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import SERVICE_NAME, Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from pydantic import Field, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict
from redis.asyncio import Redis
from temporalio.client import Client
from temporalio.contrib.opentelemetry import TracingInterceptor

from app.harness.mock_model import MockStructuredClient
from app.harness.router import ModelRouter, Price
from app.harness.skills import SkillRegistry
from app.providers.credential_crypto import CredentialOpener
from app.providers.mock import MockProvider
from app.worker.credential_activities import CredentialTestActivities
from app.worker.provider_activities import create_mock_worker
from app.worker.skill_activities import create_skill_worker


class WorkerSettings(BaseSettings):
    model_config = SettingsConfigDict(
        env_prefix="LV_", env_ignore_empty=True, extra="ignore", hide_input_in_errors=True
    )

    temporal_addr: str = Field(min_length=1)
    temporal_namespace: str = Field(min_length=1)
    redis_url: str = Field(min_length=1)
    otel_endpoint: str | None = None
    credential_key_id: str | None = None
    credential_private_key_ref: Path | None = None

    @field_validator("otel_endpoint")
    @classmethod
    def validate_otel_endpoint(cls, value: str | None) -> str | None:
        if value is None:
            return None
        value = value.strip()
        if not value:
            return None
        try:
            url = urlsplit(value)
            port = url.port
        except ValueError:
            raise ValueError("invalid LV_OTEL_ENDPOINT") from None
        if (
            (port is not None and port <= 0)
            or url.scheme not in {"http", "https"}
            or not url.hostname
            or url.username is not None
            or url.password is not None
            or url.path
            or "?" in value
            or "#" in value
            or any(ord(character) < 33 for character in value)
        ):
            raise ValueError("invalid LV_OTEL_ENDPOINT")
        return value

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


def create_trace_provider(endpoint: str | None) -> TracerProvider:
    provider = TracerProvider(resource=Resource.create({SERVICE_NAME: "lanverse-agent-worker"}))
    if endpoint:
        provider.add_span_processor(
            BatchSpanProcessor(OTLPSpanExporter(endpoint=f"{endpoint}/v1/traces", timeout=2))
        )
    return provider


async def run(settings: WorkerSettings, registry: SkillRegistry) -> None:
    router = ModelRouter(MockStructuredClient(), {"mock.structured": Price(0, 0)})
    trace_provider = create_trace_provider(settings.otel_endpoint)
    try:
        client = await Client.connect(
            settings.temporal_addr,
            namespace=settings.temporal_namespace,
            interceptors=[
                TracingInterceptor(tracer=trace_provider.get_tracer("temporal-sdk-python"))
            ],
        )
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
    finally:
        trace_provider.shutdown()


def main() -> None:
    settings = WorkerSettings()
    registry = SkillRegistry.load(Path(__file__).resolve().parent.parent / "skills")
    with suppress(KeyboardInterrupt):
        asyncio.run(run(settings, registry))


if __name__ == "__main__":
    main()
