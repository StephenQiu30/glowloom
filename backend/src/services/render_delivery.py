from __future__ import annotations

from uuid import UUID

from db.pool import DatabasePool
from repositories.deliveries import DeliveryRepository
from schemas.deliveries import DeliveryVersionSnapshot


class RenderTaskInvalid(ValueError):
    pass


class StartRenderDeliveryHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._deliveries = DeliveryRepository()

    async def execute(self, task_id: UUID) -> DeliveryVersionSnapshot:
        async with self._database.transaction() as connection:
            task_input = await self._deliveries.task_render_input(connection, task_id)
            if task_input is None:
                raise RenderTaskInvalid("render task is required")
            episode_id, snapshot_id = task_input
            if not await self._deliveries.lock_episode(connection, episode_id):
                raise RenderTaskInvalid("render task episode is missing")
            locked_input = await self._deliveries.task_render_input(connection, task_id)
            if locked_input != task_input:
                raise RenderTaskInvalid("render task input changed")
            existing = await self._deliveries.get_by_task(connection, task_id)
            if existing is not None:
                return existing
            return await self._deliveries.insert_rendering(
                connection,
                episode_id=episode_id,
                task_id=task_id,
                render_snapshot_id=snapshot_id,
            )
