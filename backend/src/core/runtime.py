from __future__ import annotations

from dataclasses import dataclass

from core.clock import Clock, SystemClock
from core.config import ApplicationSettings
from db.pool import DatabasePool
from integrations.minio_transport import MinioTransport
from integrations.object_storage import MinioObjectStore


@dataclass(frozen=True, slots=True)
class ApplicationRuntime:
    settings: ApplicationSettings
    clock: Clock
    database: DatabasePool | None
    object_store: MinioObjectStore | None

    async def start(self) -> None:
        if self.database is not None:
            await self.database.start()

    async def close(self) -> None:
        if self.database is not None:
            await self.database.close()


def create_runtime(settings: ApplicationSettings) -> ApplicationRuntime:
    database = DatabasePool.from_settings(settings) if settings.database_url else None
    object_store = None
    if settings.has_minio_configuration:
        config = settings.require_minio_config()
        transport = MinioTransport.from_credentials(
            endpoint=config.endpoint,
            access_key=config.access_key,
            secret_key=config.secret_key,
            secure=config.secure,
            public_endpoint=config.public_endpoint,
        )
        object_store = MinioObjectStore(transport, bucket=config.bucket)
    return ApplicationRuntime(
        settings=settings,
        clock=SystemClock(),
        database=database,
        object_store=object_store,
    )
