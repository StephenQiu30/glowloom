from __future__ import annotations

from dataclasses import dataclass
from typing import Any
from uuid import uuid4

from psycopg.types.json import Jsonb
from pydantic import TypeAdapter

from app.creation.contract import Command
from app.creation.repository import Repository
from app.protocol.canonical import canonical_hash, canonical_json
from app.text_contract.checks import validate_result
from app.text_contract.source import Digest
from app.text_contract.task import TextTask


class ExecutionConflict(RuntimeError):
    pass


@dataclass(frozen=True)
class InvocationLease:
    step_id: str
    command_id: str
    input_hash: str
    fence: int
    attempt_id: str


def step_key(task: TextTask) -> str:
    return "/".join(part for part in (task.stage, task.episode_key, task.scene_key) if part)


class ExecutionStore:
    def __init__(self, repository: Repository) -> None:
        self.repository = repository

    async def freeze(self, command_id: str, release_hash: str, call_limit: int) -> None:
        TypeAdapter(Digest).validate_python(release_hash)
        if isinstance(call_limit, bool) or not 1 <= call_limit <= 1000:
            raise ValueError("invalid invocation call limit")
        async with await self.repository.connect() as conn:
            await conn.execute(
                """INSERT INTO creation_executions (command_id, release_hash, call_limit)
                   VALUES (%s, %s, %s) ON CONFLICT DO NOTHING""",
                (command_id, release_hash, call_limit),
            )
            row = await (
                await conn.execute(
                    "SELECT release_hash, call_limit FROM creation_executions "
                    "WHERE command_id = %s",
                    (command_id,),
                )
            ).fetchone()
            if row != {"release_hash": release_hash, "call_limit": call_limit}:
                raise ExecutionConflict("execution_policy_conflict")

    async def reserve(self, command_id: str, task: TextTask) -> InvocationLease | dict[str, Any]:
        task = TextTask.model_validate_json(task.model_dump_json())
        raw = task.model_dump(mode="json")
        digest = canonical_hash(raw)
        expired = False
        async with await self.repository.connect() as conn:
            run = await (
                await conn.execute(
                    """SELECT e.*, c.command, c.payload_hash FROM creation_executions e
                   JOIN creation_commands c USING (command_id)
                   WHERE command_id = %s FOR UPDATE OF e""",
                    (command_id,),
                )
            ).fetchone()
            if run is None:
                raise ExecutionConflict("execution_policy_missing")
            command = Command.model_validate(run["command"])
            if (
                command.payload_hash != run["payload_hash"]
                or task.source.revision_id != command.source.revision_id
                or task.source.content_hash != command.source.content_hash
                or task.release_hash != run["release_hash"]
            ):
                raise ExecutionConflict("execution_input_mismatch")
            row = await (
                await conn.execute(
                    """SELECT *, lease_until > clock_timestamp() AS active
                   FROM creation_steps WHERE command_id = %s AND step_key = %s FOR UPDATE""",
                    (command_id, step_key(task)),
                )
            ).fetchone()
            if row:
                if row["input_hash"] != digest:
                    raise ExecutionConflict("step_input_conflict")
                if row["state"] == "needs_review":
                    saved = await (
                        await conn.execute(
                            "SELECT result, result_hash FROM creation_drafts WHERE step_id = %s",
                            (row["id"],),
                        )
                    ).fetchone()
                    if not saved or canonical_hash(saved["result"]) != saved["result_hash"]:
                        raise ExecutionConflict("persisted_draft_drift")
                    return validate_result(task, saved["result"]).model_dump(mode="json")
                if row["state"] == "unknown":
                    raise ExecutionConflict("invocation_outcome_unknown")
                if row["active"]:
                    raise ExecutionConflict("invocation_in_progress")
                if row["current_attempt_id"] is not None:
                    expired_attempt = await conn.execute(
                        """UPDATE creation_attempts SET state = 'unknown',
                           finished_at = clock_timestamp(), last_error = 'attempt_expired'
                           WHERE id = %s AND step_id = %s AND fence = %s AND state = 'running'""",
                        (row["current_attempt_id"], row["id"], row["fence"]),
                    )
                    if expired_attempt.rowcount != 1:
                        raise ExecutionConflict("attempt_fence_lost")
                await conn.execute(
                    """UPDATE creation_steps SET state = 'unknown', fence = fence + 1,
                       lease_until = NULL, last_error = 'attempt_expired',
                       updated_at = clock_timestamp()
                       WHERE id = %s""",
                    (row["id"],),
                )
                expired = True
            else:
                if run["reserved_calls"] >= run["call_limit"]:
                    raise ExecutionConflict("invocation_budget_exhausted")
                identity = str(uuid4())
                attempt_id = str(uuid4())
                await conn.execute(
                    """INSERT INTO creation_steps
                       (id, command_id, step_key, input_hash, task, state, fence, lease_until)
                       VALUES (%s, %s, %s, %s, %s, 'running', 1,
                               clock_timestamp() + %s * interval '1 second')""",
                    (
                        identity,
                        command_id,
                        step_key(task),
                        digest,
                        Jsonb(raw),
                        task.timeout_seconds + 30,
                    ),
                )
                await conn.execute(
                    """INSERT INTO creation_attempts
                       (id, step_id, attempt_no, fence, input_hash, state,
                        started_at, execution_deadline, lease_expires_at)
                       SELECT %s, id, 1, fence, input_hash, 'running', created_at,
                              lease_until - interval '30 seconds', lease_until
                       FROM creation_steps WHERE id = %s""",
                    (attempt_id, identity),
                )
                await conn.execute(
                    "UPDATE creation_steps SET current_attempt_id = %s WHERE id = %s",
                    (attempt_id, identity),
                )
                await conn.execute(
                    "UPDATE creation_executions SET reserved_calls = reserved_calls + 1 "
                    "WHERE command_id = %s",
                    (command_id,),
                )
                return InvocationLease(identity, command_id, digest, 1, attempt_id)
        # Commit the unknown transition before reporting it; never roll it back with the exception.
        assert expired
        raise ExecutionConflict("invocation_outcome_unknown")

    async def finish(self, lease: InvocationLease, raw: dict[str, Any]) -> dict[str, Any]:
        if len(canonical_json(raw)) > 8000000:
            raise ValueError("result exceeds storage limit")
        async with await self.repository.connect() as conn:
            row = await (
                await conn.execute(
                    """SELECT *, lease_until > clock_timestamp() AS active
                   FROM creation_steps WHERE id = %s AND command_id = %s FOR UPDATE""",
                    (lease.step_id, lease.command_id),
                )
            ).fetchone()
            if (
                not row
                or row["input_hash"] != lease.input_hash
                or row["fence"] != lease.fence
                or str(row["current_attempt_id"]) != lease.attempt_id
            ):
                raise ExecutionConflict("attempt_fence_lost")
            result = validate_result(TextTask.model_validate(row["task"]), raw).model_dump(
                mode="json"
            )
            result_hash = canonical_hash(result)
            if row["state"] == "needs_review":
                original = await (
                    await conn.execute(
                        "SELECT result_hash FROM creation_drafts WHERE step_id = %s",
                        (lease.step_id,),
                    )
                ).fetchone()
                if original and original["result_hash"] == result_hash:
                    return result
                raise ExecutionConflict("draft_result_conflict")
            if row["state"] != "running" or not row["active"]:
                raise ExecutionConflict("attempt_fence_lost")
            updated = await conn.execute(
                """UPDATE creation_attempts SET state = 'succeeded',
                   finished_at = clock_timestamp(), result_hash = %s
                   WHERE id = %s AND step_id = %s AND fence = %s AND input_hash = %s
                     AND state = 'running'""",
                (result_hash, lease.attempt_id, lease.step_id, lease.fence, lease.input_hash),
            )
            if updated.rowcount != 1:
                raise ExecutionConflict("attempt_fence_lost")
            draft_id = str(uuid4())
            await conn.execute(
                """INSERT INTO creation_drafts
                   (id, step_id, candidate_hash, result_hash, result, attempt_id)
                   VALUES (%s, %s, %s, %s, %s, %s)""",
                (
                    draft_id,
                    lease.step_id,
                    result["candidate_hash"],
                    result_hash,
                    Jsonb(result),
                    lease.attempt_id,
                ),
            )
            await conn.execute(
                """INSERT INTO creation_output_bindings (step_id, output_role, item_key, draft_id)
                   VALUES (%s, 'candidate', 'primary', %s)""",
                (lease.step_id, draft_id),
            )
            await conn.execute(
                """INSERT INTO creation_result_outbox
                   (event_id, step_id, draft_id, event_type, attempt_id)
                   VALUES (%s, %s, %s, 'result_ready', %s)""",
                (str(uuid4()), lease.step_id, draft_id, lease.attempt_id),
            )
            await conn.execute(
                """UPDATE creation_steps SET state = 'needs_review', lease_until = NULL,
                   updated_at = clock_timestamp() WHERE id = %s""",
                (lease.step_id,),
            )
            return result

    async def unknown(self, lease: InvocationLease, code: str) -> bool:
        if code not in {"harness_response_unknown", "harness_result_invalid", "attempt_cancelled"}:
            raise ValueError("unknown execution error code")
        async with await self.repository.connect() as conn:
            result = await conn.execute(
                """UPDATE creation_steps SET state = 'unknown', fence = fence + 1,
                   lease_until = NULL, last_error = %s, updated_at = clock_timestamp()
                   WHERE id = %s AND command_id = %s AND input_hash = %s AND fence = %s
                     AND current_attempt_id = %s
                     AND state = 'running' AND lease_until > clock_timestamp()""",
                (
                    code,
                    lease.step_id,
                    lease.command_id,
                    lease.input_hash,
                    lease.fence,
                    lease.attempt_id,
                ),
            )
            if result.rowcount != 1:
                return False
            attempt = await conn.execute(
                """UPDATE creation_attempts SET state = 'unknown',
                   finished_at = clock_timestamp(), last_error = %s
                   WHERE id = %s AND step_id = %s AND fence = %s AND input_hash = %s
                     AND state = 'running'""",
                (code, lease.attempt_id, lease.step_id, lease.fence, lease.input_hash),
            )
            if attempt.rowcount != 1:
                raise ExecutionConflict("attempt_fence_lost")
            return True

    async def attempt_history(self, command_id: str, step_id: str) -> dict[str, Any] | None:
        async with await self.repository.connect() as conn:
            # One repeatable snapshot prevents combining an old step pointer with a new result.
            await conn.execute("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY")
            step = await (
                await conn.execute(
                    "SELECT id::text, step_key, current_attempt_id::text "
                    "FROM creation_steps WHERE command_id = %s AND id = %s",
                    (command_id, step_id),
                )
            ).fetchone()
            if step is None:
                return None
            attempts = await (
                await conn.execute(
                    """SELECT id::text AS attempt_id, attempt_no, fence, input_hash, state,
                       started_at, execution_deadline, lease_expires_at, finished_at,
                       result_hash, last_error, usage_status,
                       (state = 'running' AND lease_expires_at <= transaction_timestamp())
                           AS lease_expired
                       FROM creation_attempts WHERE step_id = %s ORDER BY attempt_no""",
                    (step_id,),
                )
            ).fetchall()
            return {
                "step_id": step["id"],
                "step_key": step["step_key"],
                "current_attempt_id": step["current_attempt_id"],
                "history_origin": "recorded" if step["current_attempt_id"] else "unavailable",
                "attempts": attempts,
            }

    async def snapshot(self, command_id: str) -> dict[str, Any]:
        async with await self.repository.connect() as conn:
            row = await (
                await conn.execute(
                    "SELECT release_hash, call_limit, reserved_calls FROM creation_executions "
                    "WHERE command_id = %s",
                    (command_id,),
                )
            ).fetchone()
            if row is None:
                raise ExecutionConflict("execution_policy_missing")
            steps = await (
                await conn.execute(
                    """SELECT id::text, step_key, input_hash, state, fence, usage_status, last_error
                   FROM creation_steps WHERE command_id = %s ORDER BY created_at, id""",
                    (command_id,),
                )
            ).fetchall()
            outputs = await (
                await conn.execute(
                    """SELECT s.id::text AS step_id, s.step_key, b.output_role, b.item_key,
                          d.id::text AS draft_id, d.candidate_hash, d.result_hash
                   FROM creation_output_bindings b JOIN creation_steps s ON s.id = b.step_id
                   JOIN creation_drafts d ON d.id = b.draft_id WHERE s.command_id = %s
                   ORDER BY s.created_at, s.id""",
                    (command_id,),
                )
            ).fetchall()
            return {**row, "steps": steps, "outputs": outputs}

    async def draft(self, command_id: str, draft_id: str) -> dict[str, Any] | None:
        async with await self.repository.connect() as conn:
            row = await (
                await conn.execute(
                    """SELECT d.result, d.result_hash FROM creation_drafts d
                   JOIN creation_steps s ON s.id = d.step_id
                   WHERE s.command_id = %s AND d.id = %s""",
                    (command_id, draft_id),
                )
            ).fetchone()
            if not row:
                return None
            if canonical_hash(row["result"]) != row["result_hash"]:
                raise ExecutionConflict("persisted_draft_drift")
            return row["result"]
