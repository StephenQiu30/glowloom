from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from db.pool import DatabasePool
from repositories.render_idempotency import RenderIdempotencyRepository
from repositories.render_snapshots import RenderSnapshotRepository
from schemas.rendering import RenderRecipeV1, RenderSnapshot
from schemas.tasks import TaskAcceptedSnapshot
from services.render_inputs import RenderInputInvalid
from services.render_snapshots import (
    CreateRenderSnapshotCommand,
    CreateRenderSnapshotHandler,
    render_request_hash,
)
from services.render_task_submission import RenderTaskSubmitter


class FaultPoint(Protocol):
    def hit(self, point: str) -> None: ...


class _NoFault:
    def hit(self, point: str) -> None:
        del point


@dataclass(frozen=True, slots=True)
class RenderEpisodeCommand:
    episode_id: UUID
    idempotency_key: str


class RenderEpisodeCoordinator:
    def __init__(
        self,
        database: DatabasePool,
        *,
        recipe: RenderRecipeV1,
        release_version: str,
        fault: FaultPoint | None = None,
    ) -> None:
        self._database = database
        self._fault = fault or _NoFault()
        self._idempotency = RenderIdempotencyRepository()
        self._snapshots = RenderSnapshotRepository()
        self._snapshot_creator = CreateRenderSnapshotHandler(database, recipe=recipe)
        self._task_submitter = RenderTaskSubmitter(database, release_version=release_version)

    async def execute(self, command: RenderEpisodeCommand) -> TaskAcceptedSnapshot:
        scope = f"renderEpisode/{command.episode_id}"
        snapshot, completed = await self._tx1(command, scope)
        if completed is not None:
            return completed
        self._fault.hit("after_render_tx1")
        task = await self._task_submitter.submit(command.episode_id, snapshot.id)
        self._fault.hit("after_render_tx2")
        await self._tx3(command, scope, snapshot, task)
        self._fault.hit("after_render_tx3")
        return task

    async def _tx1(
        self,
        command: RenderEpisodeCommand,
        scope: str,
    ) -> tuple[RenderSnapshot, TaskAcceptedSnapshot | None]:
        request_hash = render_request_hash(command.episode_id)
        async with self._database.transaction() as connection:
            locked = await connection.fetchval(
                "SELECT true FROM episodes WHERE id=$1 FOR UPDATE", command.episode_id
            )
            if locked is not True:
                raise RenderInputInvalid("episode does not exist")
            record = await self._idempotency.reserve(
                connection,
                scope=scope,
                key=command.idempotency_key,
                request_hash=request_hash,
                request_id=new_id(),
            )
            if record.state == "completed":
                if record.reference is None:
                    raise RuntimeError("completed render response is missing")
                snapshot = await self._snapshot_from_reference(connection, record.reference)
                return snapshot, TaskAcceptedSnapshot(
                    task_id=UUID(str(record.reference["task_id"])),
                    snapshot_id=UUID(str(record.reference["snapshot_id"])),
                )
            snapshot = await self._pending_snapshot(connection, command, scope, record.reference)
            await self._idempotency.set_snapshot_reference(
                connection,
                scope=scope,
                key=command.idempotency_key,
                snapshot_id=snapshot.id,
            )
            return snapshot, None

    async def _tx3(
        self,
        command: RenderEpisodeCommand,
        scope: str,
        snapshot: RenderSnapshot,
        task: TaskAcceptedSnapshot,
    ) -> None:
        async with self._database.transaction() as connection:
            await connection.fetchval(
                "SELECT true FROM episodes WHERE id=$1 FOR UPDATE", command.episode_id
            )
            current = await self._snapshots.get(connection, snapshot.id, for_update=True)
            if current is None or current.episode_id != command.episode_id:
                raise RuntimeError("render snapshot is missing")
            await self._snapshots.bind_initial_task(
                connection, snapshot_id=snapshot.id, task_id=task.task_id
            )
            await self._idempotency.complete(
                connection,
                scope=scope,
                key=command.idempotency_key,
                task_id=task.task_id,
                submission_snapshot_id=task.snapshot_id,
                render_snapshot_id=snapshot.id,
            )

    async def _pending_snapshot(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        command: RenderEpisodeCommand,
        scope: str,
        reference: dict[str, object] | None,
    ) -> RenderSnapshot:
        if reference is not None:
            return await self._snapshot_from_reference(connection, reference)
        return await self._snapshot_creator.create_in_transaction(
            connection,
            CreateRenderSnapshotCommand(command.episode_id, scope, command.idempotency_key),
        )

    async def _snapshot_from_reference(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        reference: dict[str, object],
    ) -> RenderSnapshot:
        snapshot = await self._snapshots.get(connection, UUID(str(reference["render_snapshot_id"])))
        if snapshot is None:
            raise RuntimeError("render response references a missing snapshot")
        return snapshot
