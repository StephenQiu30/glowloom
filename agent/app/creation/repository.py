from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from typing import Any
from uuid import uuid4

from psycopg import AsyncConnection
from psycopg.rows import dict_row
from psycopg.types.json import Jsonb

from app.creation.contract import Acceptance, Command

# This schema belongs exclusively to the Agent database. Migration is explicit.
SCHEMA = """
CREATE TABLE creation_commands (
    command_id uuid PRIMARY KEY,
    run_id uuid NOT NULL UNIQUE,
    workflow_id text NOT NULL UNIQUE,
    payload_hash text NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    command jsonb NOT NULL,
    receipt jsonb NOT NULL,
    accepted_at timestamptz NOT NULL
);
CREATE TABLE creation_start_outbox (
    command_id uuid PRIMARY KEY REFERENCES creation_commands(command_id),
    task_queue text NOT NULL CHECK (length(task_queue) BETWEEN 1 AND 255),
    state text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'started', 'blocked')),
    attempts integer NOT NULL DEFAULT 0,
    fence bigint NOT NULL DEFAULT 0,
    lease_until timestamptz,
    next_attempt_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    temporal_run_id uuid,
    last_error text,
    CHECK ((state = 'started') = (temporal_run_id IS NOT NULL))
);
CREATE INDEX creation_start_due ON creation_start_outbox(next_attempt_at)
    WHERE state = 'pending';
"""
SCHEMA_HASH = hashlib.sha256(SCHEMA.encode()).hexdigest()


class CommandConflict(ValueError):
    pass


class SchemaMismatch(RuntimeError):
    pass


@dataclass(frozen=True)
class StartLease:
    command: Command
    task_queue: str
    fence: int
    attempts: int
    start_allowed: bool


class Repository:
    def __init__(self, dsn: str) -> None:
        self.dsn = dsn

    async def connect(self) -> AsyncConnection[dict[str, Any]]:
        return await AsyncConnection[dict[str, Any]].connect(
            self.dsn,
            row_factory=dict_row,
            connect_timeout=5,
            options="-c search_path=public -c statement_timeout=5000 -c lock_timeout=5000",
        )

    async def migrate(self) -> None:
        async with await self.connect() as conn:
            await conn.execute("SELECT pg_advisory_xact_lock(764132009)")
            marker = await (
                await conn.execute("SELECT to_regclass('public.creation_schema') AS marker")
            ).fetchone()
            if marker and marker["marker"]:
                await self._check_schema(conn)
                return
            existing = await (
                await conn.execute(
                    "SELECT tablename FROM pg_tables WHERE schemaname = 'public' LIMIT 1"
                )
            ).fetchone()
            if existing:
                raise SchemaMismatch(
                    "initial creation migration requires an empty dedicated database"
                )
            await conn.execute(
                "CREATE TABLE creation_schema (name text PRIMARY KEY, checksum text NOT NULL)"
            )
            await conn.execute(SCHEMA)
            await conn.execute(
                "INSERT INTO creation_schema VALUES (%s, %s)", ("command-acceptance", SCHEMA_HASH)
            )

    async def _check_schema(self, conn: AsyncConnection[dict[str, Any]]) -> None:
        row = await (
            await conn.execute(
                "SELECT checksum FROM creation_schema WHERE name = 'command-acceptance'"
            )
        ).fetchone()
        if not row or row["checksum"] != SCHEMA_HASH:
            raise SchemaMismatch("creation database migration does not match this application")

    async def ready(self) -> None:
        async with await self.connect() as conn:
            await self._check_schema(conn)

    async def accept(self, command: Command, task_queue: str) -> Acceptance:
        async with await self.connect() as conn:
            instant = await (await conn.execute("SELECT clock_timestamp() AS instant")).fetchone()
            assert instant is not None
            receipt = Acceptance.for_command(command, str(uuid4()), instant["instant"])
            inserted = await (
                await conn.execute(
                    """INSERT INTO creation_commands
                   (command_id, run_id, workflow_id, payload_hash, command, receipt, accepted_at)
                   VALUES (%s, %s, %s, %s, %s, %s, %s)
                   ON CONFLICT DO NOTHING RETURNING command_id""",
                    (
                        command.command_id,
                        command.run_id,
                        command.workflow_id,
                        command.payload_hash,
                        Jsonb(command.model_dump(mode="json", by_alias=True)),
                        Jsonb(receipt.model_dump(mode="json", by_alias=True)),
                        receipt.accepted_at,
                    ),
                )
            ).fetchone()
            if inserted:
                await conn.execute(
                    "INSERT INTO creation_start_outbox (command_id, task_queue) VALUES (%s, %s)",
                    (command.command_id, task_queue),
                )
                return receipt
            original = await (
                await conn.execute(
                    "SELECT payload_hash, receipt FROM creation_commands WHERE command_id = %s",
                    (command.command_id,),
                )
            ).fetchone()
            if not original or original["payload_hash"] != command.payload_hash:
                raise CommandConflict("creation command identity has different content")
            return Acceptance.model_validate_json(json.dumps(original["receipt"]))

    async def get(self, command_id: str) -> Acceptance | None:
        async with await self.connect() as conn:
            row = await (
                await conn.execute(
                    "SELECT receipt FROM creation_commands WHERE command_id = %s",
                    (command_id,),
                )
            ).fetchone()
            return Acceptance.model_validate_json(json.dumps(row["receipt"])) if row else None

    async def claim(self) -> StartLease | None:
        async with await self.connect() as conn:
            row = await (
                await conn.execute(
                    """WITH due AS (
                    SELECT command_id FROM creation_start_outbox
                    WHERE state = 'pending' AND next_attempt_at <= clock_timestamp()
                      AND (lease_until IS NULL OR lease_until <= clock_timestamp())
                    ORDER BY next_attempt_at, command_id FOR UPDATE SKIP LOCKED LIMIT 1
                ), claimed AS (
                    UPDATE creation_start_outbox o SET fence = fence + 1,
                      attempts = attempts + 1,
                      lease_until = clock_timestamp() + interval '45 seconds'
                    FROM due WHERE o.command_id = due.command_id RETURNING o.*
                ) SELECT c.command, o.task_queue, o.fence, o.attempts,
                    c.accepted_at > clock_timestamp() - interval '12 hours' AS start_allowed
                  FROM claimed o
                  JOIN creation_commands c USING (command_id)"""
                )
            ).fetchone()
            if not row:
                return None
            return StartLease(
                Command.model_validate(row["command"]),
                row["task_queue"],
                row["fence"],
                row["attempts"],
                row["start_allowed"],
            )

    async def finish(self, lease: StartLease, *, temporal_run_id: str) -> bool:
        async with await self.connect() as conn:
            cursor = await conn.execute(
                """UPDATE creation_start_outbox SET state = 'started', temporal_run_id = %s,
                   lease_until = NULL, last_error = NULL
                   WHERE command_id = %s AND fence = %s AND state = 'pending'
                   AND lease_until > clock_timestamp()""",
                (temporal_run_id, lease.command.command_id, lease.fence),
            )
            return cursor.rowcount == 1

    async def fail(self, lease: StartLease, code: str, *, blocked: bool = False) -> bool:
        async with await self.connect() as conn:
            cursor = await conn.execute(
                """UPDATE creation_start_outbox SET state = %s, last_error = %s,
                   lease_until = NULL,
                   next_attempt_at = clock_timestamp() + %s * interval '1 second'
                   WHERE command_id = %s AND fence = %s AND state = 'pending'
                   AND lease_until > clock_timestamp()""",
                (
                    "blocked" if blocked else "pending",
                    code,
                    2 ** min(lease.attempts, 8),
                    lease.command.command_id,
                    lease.fence,
                ),
            )
            return cursor.rowcount == 1
