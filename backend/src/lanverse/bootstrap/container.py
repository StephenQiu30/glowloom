from __future__ import annotations

from dataclasses import dataclass

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.shared_kernel.clock import Clock, SystemClock
from lanverse.shared_kernel.config import ApplicationSettings


@dataclass(frozen=True, slots=True)
class ApplicationContainer:
    settings: ApplicationSettings
    clock: Clock
    database: DatabasePool | None

    async def start(self) -> None:
        if self.database is not None:
            await self.database.start()

    async def close(self) -> None:
        if self.database is not None:
            await self.database.close()


def create_container(settings: ApplicationSettings) -> ApplicationContainer:
    database = DatabasePool.from_settings(settings) if settings.database_url else None
    return ApplicationContainer(settings=settings, clock=SystemClock(), database=database)
