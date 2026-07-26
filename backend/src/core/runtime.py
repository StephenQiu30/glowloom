from __future__ import annotations

from dataclasses import dataclass

from core.clock import Clock, SystemClock
from core.config import ApplicationSettings
from db.pool import DatabasePool


@dataclass(frozen=True, slots=True)
class ApplicationRuntime:
    settings: ApplicationSettings
    clock: Clock
    database: DatabasePool | None

    async def start(self) -> None:
        if self.database is not None:
            await self.database.start()

    async def close(self) -> None:
        if self.database is not None:
            await self.database.close()


def create_runtime(settings: ApplicationSettings) -> ApplicationRuntime:
    database = DatabasePool.from_settings(settings) if settings.database_url else None
    return ApplicationRuntime(settings=settings, clock=SystemClock(), database=database)
