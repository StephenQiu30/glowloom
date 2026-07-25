from __future__ import annotations

import json
from dataclasses import dataclass
from datetime import datetime, timedelta
from typing import Any
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]


@dataclass(frozen=True, slots=True)
class JobLease:
    id: UUID
    task_id: UUID
    payload: dict[str, object]
    lease_owner: str
    lease_until: datetime
    attempts: int


def payload_value(raw: Any) -> dict[str, object]:
    if isinstance(raw, str):
        raw = json.loads(raw)
    if not isinstance(raw, dict):
        raise RuntimeError("TaskJob payload must be an object")
    return {str(key): value for key, value in raw.items()}


class JobLeaseRepository:
    async def claim(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        owner: str,
        now: datetime,
        lease_duration: timedelta,
    ) -> JobLease | None:
        row = await connection.fetchrow(
            """
            WITH candidate AS (
                SELECT j.id
                FROM task_jobs j
                JOIN production_tasks t ON t.id = j.task_id
                WHERE (
                    (j.state = 'pending' AND j.next_attempt_at <= $2)
                    OR (j.state = 'leased' AND j.lease_until <= $2)
                )
                AND t.status IN ('queued','running','cancelling','unknown')
                ORDER BY j.next_attempt_at, j.created_at, j.id
                FOR UPDATE OF j SKIP LOCKED
                LIMIT 1
            )
            UPDATE task_jobs j
            SET state='leased', lease_owner=$1,
                lease_until=$2::timestamptz + $3::interval,
                attempts=j.attempts + 1, updated_at=$2
            FROM candidate c WHERE j.id=c.id
            RETURNING j.*
            """,
            owner,
            now,
            lease_duration,
        )
        if row is None:
            return None
        return JobLease(
            id=row["id"],
            task_id=row["task_id"],
            payload=payload_value(row["payload_json"]),
            lease_owner=row["lease_owner"],
            lease_until=row["lease_until"],
            attempts=row["attempts"],
        )

    async def heartbeat(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        job_id: UUID,
        owner: str,
        now: datetime,
        lease_duration: timedelta,
    ) -> bool:
        result = await connection.execute(
            """
            UPDATE task_jobs
            SET lease_until=$3::timestamptz + $4::interval, updated_at=$3
            WHERE id=$1 AND state='leased' AND lease_owner=$2 AND lease_until > $3
            """,
            job_id,
            owner,
            now,
            lease_duration,
        )
        return str(result) == "UPDATE 1"

    async def complete(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        job_id: UUID,
        owner: str,
        now: datetime,
    ) -> bool:
        result = await connection.execute(
            """
            UPDATE task_jobs
            SET state='completed', lease_owner=NULL, lease_until=NULL,
                updated_at=$3, completed_at=$3
            WHERE id=$1 AND state='leased' AND lease_owner=$2 AND lease_until > $3
            """,
            job_id,
            owner,
            now,
        )
        return str(result) == "UPDATE 1"

    async def release(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        *,
        job_id: UUID,
        owner: str,
        now: datetime,
        next_attempt_at: datetime,
        error_code: str,
    ) -> bool:
        result = await connection.execute(
            """
            UPDATE task_jobs
            SET state='pending', lease_owner=NULL, lease_until=NULL,
                updated_at=$3, next_attempt_at=$4, last_error_code=$5
            WHERE id=$1 AND state='leased' AND lease_owner=$2
            """,
            job_id,
            owner,
            now,
            next_attempt_at,
            error_code,
        )
        return str(result) == "UPDATE 1"
