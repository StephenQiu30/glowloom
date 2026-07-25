from __future__ import annotations

from datetime import datetime, timedelta
from uuid import UUID

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.modules.production_jobs.infrastructure.leases import (
    JobLease,
    JobLeaseRepository,
)


class JobQueue:
    def __init__(self, database: DatabasePool, *, lease_duration: timedelta) -> None:
        if lease_duration <= timedelta(0):
            raise ValueError("lease_duration must be positive")
        self._database = database
        self._lease_duration = lease_duration
        self._leases = JobLeaseRepository()

    async def claim(self, owner: str, *, now: datetime) -> JobLease | None:
        async with self._database.transaction() as connection:
            return await self._leases.claim(
                connection,
                owner=owner,
                now=now,
                lease_duration=self._lease_duration,
            )

    async def heartbeat(self, job_id: UUID, owner: str, *, now: datetime) -> bool:
        async with self._database.transaction() as connection:
            return await self._leases.heartbeat(
                connection,
                job_id=job_id,
                owner=owner,
                now=now,
                lease_duration=self._lease_duration,
            )

    async def complete(self, job_id: UUID, owner: str, *, now: datetime) -> bool:
        async with self._database.transaction() as connection:
            return await self._leases.complete(
                connection, job_id=job_id, owner=owner, now=now
            )

    async def release(
        self,
        job_id: UUID,
        owner: str,
        *,
        now: datetime,
        next_attempt_at: datetime,
        error_code: str,
    ) -> bool:
        async with self._database.transaction() as connection:
            return await self._leases.release(
                connection,
                job_id=job_id,
                owner=owner,
                now=now,
                next_attempt_at=next_attempt_at,
                error_code=error_code,
            )
