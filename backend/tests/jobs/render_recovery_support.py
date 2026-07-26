from __future__ import annotations

from datetime import timedelta

from db.pool import DatabasePool
from integrations.ffmpeg_recipe import RenderSources
from integrations.ffmpeg_render import RenderRuntimeError
from integrations.object_storage import MinioObjectStore
from schemas.rendering import RenderRecipeV1
from services.render_submission import RenderEpisodeCommand, RenderEpisodeCoordinator
from tests.integration.delivery.render_fixture import (
    SuccessfulRenderRuntime,
    ValidDeliveryProbeRuntime,
    render_recipe,
)
from tests.integration.media_library.support import MemoryTransport
from workers.dispatch import JobHandlerRegistry
from workers.provider_execution import FaultInjector, InjectedFault
from workers.render_episode import RenderEpisodeJobHandler
from workers.runner import TaskJobRunner


class FailOnce(FaultInjector):
    def __init__(self, point: str) -> None:
        self._point = point
        self._triggered = False

    def hit(self, point: str) -> None:
        if point == self._point and not self._triggered:
            self._triggered = True
            raise InjectedFault(point)


class CountingRuntime(SuccessfulRenderRuntime):
    def __init__(self) -> None:
        self.calls = 0

    async def render(self, sources: RenderSources, recipe: RenderRecipeV1) -> bytes:
        self.calls += 1
        return await super().render(sources, recipe)


class FailsThenSucceeds(CountingRuntime):
    def __init__(self, failures: int) -> None:
        super().__init__()
        self._failures = failures

    async def render(self, sources: RenderSources, recipe: RenderRecipeV1) -> bytes:
        self.calls += 1
        if self.calls <= self._failures:
            raise RenderRuntimeError("temporary FFmpeg failure")
        return b"delivery-api-final-mp4"


def runner(
    database: DatabasePool,
    transport: MemoryTransport,
    runtime: CountingRuntime,
    fault: FaultInjector,
    owner: str,
) -> TaskJobRunner:
    registry = JobHandlerRegistry()
    registry.register(
        task_type="render_episode",
        release_version="test-release",
        handler_version="render-v1",
        handler=RenderEpisodeJobHandler(
            database,
            object_store=MinioObjectStore(transport, bucket="lanverse"),
            render_runtime=runtime,
            probe_runtime=ValidDeliveryProbeRuntime(),
            fault=fault,
        ),
    )
    return TaskJobRunner(
        database,
        registry=registry,
        owner=owner,
        lease_duration=timedelta(seconds=10),
        fault=fault,
    )


async def submit_render(database: DatabasePool, episode_id, key: str):
    async with database.transaction() as connection:
        await connection.execute(
            """
            UPDATE task_jobs job SET state='completed',completed_at=now(),updated_at=now()
            FROM production_tasks task WHERE task.id=job.task_id
              AND task.episode_id=$1 AND task.type<>'render_episode'
              AND job.state='pending'
            """,
            episode_id,
        )
    return await RenderEpisodeCoordinator(
        database,
        recipe=render_recipe(),
        release_version="test-release",
        fault=FaultInjector(),
    ).execute(RenderEpisodeCommand(episode_id, key))


async def render_counts(database: DatabasePool, task_id):
    async with database.transaction() as connection:
        row = await connection.fetchrow(
            """
            SELECT task.status task_status,delivery.status delivery_status,job.state,
                   (SELECT count(*) FROM production_attempts WHERE task_id=task.id) attempts,
                   (SELECT count(*) FROM media_versions version JOIN production_attempts item
                    ON item.id=version.origin_attempt_id WHERE item.task_id=task.id) artifacts
            FROM production_tasks task JOIN task_jobs job ON job.task_id=task.id
            JOIN delivery_versions delivery ON delivery.render_task_id=task.id
            WHERE task.id=$1
            """,
            task_id,
        )
    assert row is not None
    return tuple(row)


async def attempt_states(database: DatabasePool, task_id):
    async with database.transaction() as connection:
        rows = await connection.fetch(
            """
            SELECT attempt_no,status,
                   (SELECT attempt_no FROM production_attempts parent
                    WHERE parent.id=item.parent_attempt_id) parent_no
            FROM production_attempts item WHERE task_id=$1 ORDER BY attempt_no
            """,
            task_id,
        )
    return tuple(tuple(row) for row in rows)


async def delivery_retry_chain(database: DatabasePool, original_task_id, retry_task_id):
    async with database.transaction() as connection:
        rows = await connection.fetch(
            """
            SELECT id,render_task_id,status,retry_of_delivery_id FROM delivery_versions
            WHERE render_task_id=ANY($1::uuid[]) ORDER BY version
            """,
            [original_task_id, retry_task_id],
        )
    assert len(rows) == 2
    return rows[0]["status"], rows[1]["status"], rows[1]["retry_of_delivery_id"] == rows[0]["id"]
