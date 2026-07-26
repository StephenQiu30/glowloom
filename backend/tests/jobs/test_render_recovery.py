from __future__ import annotations

from datetime import UTC, datetime, timedelta

import pytest

from db.pool import DatabasePool
from services.tasks import RetryTaskHandler, TaskQueryService
from tests.integration.delivery.support import render_ready_story
from tests.jobs.render_recovery_support import (
    CountingRuntime,
    FailOnce,
    FailsThenSucceeds,
    attempt_states,
    delivery_retry_chain,
    render_counts,
    runner,
    submit_render,
)
from workers.provider_execution import FaultInjector, InjectedFault

NOW = datetime(2030, 7, 25, 12, 0, tzinfo=UTC)


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
            assert await runner(database, transport, runtime, fault, "worker-b").run_once(
                now=NOW + timedelta(minutes=index, seconds=11)
            )
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
