"""Explicit one-attempt recovery grants; never an automatic retry loop."""

from typing import Any
from uuid import uuid4

from psycopg import AsyncConnection

from app.creation.contract import canonical_uuid
from app.creation.execution import ExecutionConflict, InvocationLease
from app.creation.repository import Repository
from app.text_contract.task import TextTask


class RecoveryStore:
    def __init__(self, repository: Repository) -> None:
        self.repository = repository

    async def authorize(
        self,
        request_id: str,
        command_id: str,
        step_id: str,
        attempt_id: str,
        temporal_run_id: str,
        reset_event_id: int,
        reason: str,
    ) -> None:
        for value in (request_id, command_id, step_id, attempt_id, temporal_run_id):
            canonical_uuid(value)
        if not 10 <= len(reason) <= 500 or isinstance(reset_event_id, bool) or reset_event_id < 1:
            raise ValueError("recovery authorization invalid")
        async with await self.repository.connect() as conn:
            run = await (
                await conn.execute(
                    "SELECT * FROM creation_executions WHERE command_id=%s FOR UPDATE",
                    (command_id,),
                )
            ).fetchone()
            old = await (
                await conn.execute("SELECT * FROM creation_recoveries WHERE id=%s", (request_id,))
            ).fetchone()
            if old:
                if (
                    str(old["command_id"]),
                    str(old["step_id"]),
                    str(old["previous_attempt_id"]),
                    str(old["temporal_run_id"]),
                    old["reset_event_id"],
                    old["reason"],
                ) != (command_id, step_id, attempt_id, temporal_run_id, reset_event_id, reason):
                    raise ExecutionConflict("recovery_request_conflict")
                return
            step = await (
                await conn.execute(
                    "SELECT * FROM creation_steps WHERE command_id=%s AND id=%s FOR UPDATE",
                    (command_id, step_id),
                )
            ).fetchone()
            if (
                not run
                or run["status"] != "blocked"
                or not step
                or step["state"] not in {"unknown", "failed"}
                or str(step["current_attempt_id"]) != attempt_id
            ):
                raise ExecutionConflict("recovery_target_invalid")
            if run["reserved_calls"] >= run["call_limit"]:
                raise ExecutionConflict("invocation_budget_exhausted")
            existing = await (
                await conn.execute(
                    "SELECT id FROM creation_recoveries WHERE previous_attempt_id=%s", (attempt_id,)
                )
            ).fetchone()
            if existing:
                raise ExecutionConflict("recovery_already_authorized")
            await conn.execute(
                """INSERT INTO creation_recoveries
                   (id,command_id,step_id,previous_attempt_id,input_hash,temporal_run_id,reset_event_id,reason)
                   VALUES (%s,%s,%s,%s,%s,%s,%s,%s)""",
                (
                    request_id,
                    command_id,
                    step_id,
                    attempt_id,
                    step["input_hash"],
                    temporal_run_id,
                    reset_event_id,
                    reason,
                ),
            )

    async def get(self, request_id: str) -> dict[str, Any] | None:
        async with await self.repository.connect() as conn:
            return await (
                await conn.execute("SELECT * FROM creation_recoveries WHERE id=%s", (request_id,))
            ).fetchone()

    async def record_reset(self, request_id: str, run_id: str) -> None:
        canonical_uuid(run_id)
        async with await self.repository.connect() as conn:
            changed = await conn.execute(
                """UPDATE creation_recoveries SET reset_run_id=%s WHERE id=%s AND
                   (reset_run_id IS NULL OR reset_run_id=%s)""",
                (run_id, request_id, run_id),
            )
            if changed.rowcount != 1:
                raise ExecutionConflict("recovery_reset_conflict")


async def claim_recovery(
    conn: AsyncConnection[dict[str, Any]], run: dict[str, Any], step: dict[str, Any], task: TextTask
) -> InvocationLease:
    grant = await (
        await conn.execute(
            """SELECT * FROM creation_recoveries WHERE command_id=%s AND step_id=%s AND
                   previous_attempt_id=%s AND input_hash=%s AND consumed_at IS NULL FOR UPDATE""",
            (run["command_id"], step["id"], step["current_attempt_id"], step["input_hash"]),
        )
    ).fetchone()
    if not grant:
        raise ExecutionConflict(
            "invocation_outcome_unknown" if step["state"] == "unknown" else step["last_error"]
        )
    if run["reserved_calls"] >= run["call_limit"]:
        raise ExecutionConflict("invocation_budget_exhausted")
    previous = await (
        await conn.execute(
            "SELECT attempt_no FROM creation_attempts WHERE id=%s", (step["current_attempt_id"],)
        )
    ).fetchone()
    if not previous:
        raise ExecutionConflict("recovery_target_invalid")
    attempt_id = str(uuid4())
    fence = step["fence"] + 1
    await conn.execute(
        """INSERT INTO creation_attempts
                   (id,step_id,attempt_no,fence,input_hash,state,started_at,execution_deadline,lease_expires_at)
                   VALUES
                   (%s,%s,%s,%s,%s,'running',transaction_timestamp(),transaction_timestamp()+%s*interval
                   '1 second',transaction_timestamp()+%s*interval '1 second')""",
        (
            attempt_id,
            step["id"],
            previous["attempt_no"] + 1,
            fence,
            step["input_hash"],
            task.timeout_seconds,
            task.timeout_seconds + 30,
        ),
    )
    await conn.execute(
        """UPDATE creation_steps SET
                   state='running',fence=%s,current_attempt_id=%s,lease_until=transaction_timestamp()+%s*interval
                   '1 second',last_error=NULL,updated_at=clock_timestamp() WHERE id=%s""",
        (fence, attempt_id, task.timeout_seconds + 30, step["id"]),
    )
    await conn.execute(
        "UPDATE creation_recoveries SET consumed_at=clock_timestamp() WHERE id=%s", (grant["id"],)
    )
    await conn.execute(
        "UPDATE creation_executions SET reserved_calls=reserved_calls+1 WHERE command_id=%s",
        (run["command_id"],),
    )
    return InvocationLease(
        str(step["id"]), str(run["command_id"]), step["input_hash"], fence, attempt_id
    )
