from __future__ import annotations

from datetime import UTC, datetime, timedelta

import pytest

from db.pool import DatabasePool
from integrations.ffmpeg_recipe import RenderSources
from integrations.ffmpeg_render import RenderRuntimeError
from integrations.object_storage import MinioObjectStore
from schemas.rendering import RenderRecipeV1
from services.render_submission import RenderEpisodeCommand, RenderEpisodeCoordinator
from services.tasks import RetryTaskHandler, TaskQueryService
from tests.integration.delivery.render_fixture import (
    SuccessfulRenderRuntime,
    ValidDeliveryProbeRuntime,
    render_recipe,
)
from tests.integration.delivery.support import render_ready_story
from tests.integration.media_library.support import MemoryTransport
from workers.dispatch import JobHandlerRegistry
from workers.provider_execution import FaultInjector, InjectedFault
from workers.render_episode import RenderEpisodeJobHandler
from workers.runner import TaskJobRunner

NOW = datetime(2030, 7, 25, 12, 0, tzinfo=UTC)


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
        handler_version="1",
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
    return await RenderEpisodeCoordinator(
        database,
        recipe=render_recipe(),
        release_version="test-release",
        fault=FaultInjector(),
    ).execute(RenderEpisodeCommand(episode_id, key))


@pytest.mark.asyncio
async def test_render_recovers_each_process_crash_without_duplicate_facts(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=12)
    await database.start()
    try:
        episode_id, _, _, transport = await render_ready_story(database, "render-crashes")
        for index, point in enumerate(
            (
                "before_ffmpeg",
                "after_ffmpeg",
                "before_artifact_upload",
                "after_artifact_upload",
                "after_delivery_ready",
            )
        ):
            accepted = await submit_render(database, episode_id, f"crash:{point}")
            runtime = CountingRuntime()
            fault = FailOnce(point)
            with pytest.raises(InjectedFault, match=point):
                await runner(database, transport, runtime, fault, "worker-a").run_once(
                    now=NOW + timedelta(minutes=index)
                )
            assert await runner(
                database, transport, runtime, fault, "worker-b"
            ).run_once(now=NOW + timedelta(minutes=index, seconds=11))
            assert await render_counts(database, accepted.task_id) == (
                "succeeded",
                "ready",
                "completed",
                1,
                3,
            )
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_retryable_ffmpeg_failure_uses_three_parented_attempts(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=12)
    await database.start()
    try:
        episode_id, _, _, transport = await render_ready_story(database, "render-auto-retry")
        accepted = await submit_render(database, episode_id, "retry:auto")
        runtime = FailsThenSucceeds(2)
        worker = runner(database, transport, runtime, FaultInjector(), "retry-worker")
        assert await worker.run_once(now=NOW)
        assert await worker.run_once(now=NOW + timedelta(seconds=2))
        assert await worker.run_once(now=NOW + timedelta(seconds=4))
        assert await attempt_states(database, accepted.task_id) == (
            (1, "failed", None),
            (2, "failed", 1),
            (3, "succeeded", 2),
        )
        assert (await render_counts(database, accepted.task_id))[:3] == (
            "succeeded",
            "ready",
            "completed",
        )
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_exhausted_render_can_be_user_retried_without_rewriting_old_delivery(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=12)
    await database.start()
    try:
        episode_id, _, _, transport = await render_ready_story(database, "render-user-retry")
        original = await submit_render(database, episode_id, "retry:exhaust")
        worker = runner(database, transport, FailsThenSucceeds(3), FaultInjector(), "fail")
        for seconds in (0, 2, 4):
            assert await worker.run_once(now=NOW + timedelta(seconds=seconds))
        failed = await TaskQueryService(database).get(original.task_id)
        assert failed.status == "failed" and failed.error == {
            "retryable": True,
            "summary": "temporary FFmpeg failure",
        }

        retried = await RetryTaskHandler(database, release_version="test-release").execute(
            original.task_id,
            failed.resource_version,
            "retry:render:user:0001",
        )
        assert await runner(
            database, transport, CountingRuntime(), FaultInjector(), "success"
        ).run_once(now=NOW + timedelta(seconds=6))
        assert await delivery_retry_chain(database, original.task_id, retried.task_id) == (
            "failed",
            "ready",
            True,
        )
    finally:
        await database.close()


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
