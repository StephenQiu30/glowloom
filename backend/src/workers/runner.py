from __future__ import annotations

from datetime import datetime, timedelta

from db.pool import DatabasePool
from schemas.jobs import InvalidJobPayload, JobPayload
from workers.capacity import WorkerCapacity
from workers.dispatch import JobContext, JobHandlerRegistry
from workers.errors import RetryableJobError
from workers.lease_queue import JobQueue
from workers.provider_execution import FaultInjector


class TaskJobRunner:
    def __init__(
        self,
        database: DatabasePool,
        *,
        registry: JobHandlerRegistry,
        owner: str,
        lease_duration: timedelta,
        fault: FaultInjector,
        capacity: WorkerCapacity | None = None,
    ) -> None:
        self._database = database
        self._registry = registry
        self._owner = owner
        self._queue = JobQueue(database, lease_duration=lease_duration)
        self._fault = fault
        self._capacity = capacity or WorkerCapacity(limit=3)

    async def run_once(self, *, now: datetime) -> bool:
        if not await self._capacity.try_acquire():
            return False
        try:
            lease = await self._queue.claim(self._owner, now=now)
            if lease is None:
                return False
            self._fault.hit("after_claim")
            payload = JobPayload.parse(lease.payload)
            if payload.task_id != lease.task_id:
                raise InvalidJobPayload("TaskJob task_id does not match its row")
            async with self._database.transaction() as connection:
                row = await connection.fetchrow(
                    "SELECT type,snapshot_id FROM production_tasks WHERE id=$1",
                    payload.task_id,
                )
            if row is None or row["snapshot_id"] != payload.snapshot_id:
                raise InvalidJobPayload("TaskJob snapshot_id does not match its task")
            context = JobContext(job_id=lease.id, owner=self._owner, payload=payload)
            try:
                await self._registry.dispatch(row["type"], context)
            except RetryableJobError as error:
                released = await self._queue.release(
                    lease.id,
                    self._owner,
                    now=now,
                    next_attempt_at=now + error.retry_after,
                    error_code=error.code,
                )
                if not released:
                    raise RuntimeError("worker lost the lease before retry release") from error
                return True
            completed = await self._queue.complete(lease.id, self._owner, now=now)
            if not completed:
                raise RuntimeError("worker lost the lease before completion")
            return True
        finally:
            await self._capacity.release()
