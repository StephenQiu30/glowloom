from __future__ import annotations

import asyncio
import json
from uuid import UUID

import pytest

from db.pool import DatabasePool
from integrations.ai.deterministic_video import FFMPEG_IMAGE
from schemas.rendering import RenderRecipeV1
from services.render_delivery import StartRenderDeliveryHandler
from services.render_submission import RenderEpisodeCommand, RenderEpisodeCoordinator
from tests.integration.delivery.support import render_ready_story
from workers.provider_execution import FaultInjector, InjectedFault


class FailOnce(FaultInjector):
    def __init__(self, point: str) -> None:
        self._point = point
        self._failed = False

    def hit(self, point: str) -> None:
        if point == self._point and not self._failed:
            self._failed = True
            raise InjectedFault(point)


def recipe() -> RenderRecipeV1:
    return RenderRecipeV1(
        runtime_image=FFMPEG_IMAGE,
        ffmpeg_version="mock-ffmpeg-7.1",
        ffprobe_version="mock-ffprobe-7.1",
        font_name="Noto Sans CJK SC",
        font_file="/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
        font_sha256="a" * 64,
        font_license="OFL-1.1",
    )


@pytest.mark.parametrize(
    ("fault_point", "first_task_count", "first_bound", "first_state"),
    (
        ("after_render_tx1", 0, False, "pending"),
        ("after_render_tx2", 1, False, "pending"),
        ("after_render_tx3", 1, True, "completed"),
    ),
)
@pytest.mark.asyncio
async def test_render_submission_replays_each_committed_fault_boundary(
    migrated_database_url: str,
    fault_point: str,
    first_task_count: int,
    first_bound: bool,
    first_state: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=12)
    await database.start()
    try:
        episode_id, _, _, _ = await render_ready_story(database, fault_point)
        command = RenderEpisodeCommand(episode_id, f"render:submit:{fault_point}")
        coordinator = RenderEpisodeCoordinator(
            database,
            recipe=recipe(),
            release_version="test-release",
            fault=FailOnce(fault_point),
        )

        with pytest.raises(InjectedFault, match=fault_point):
            await coordinator.execute(command)

        first = await render_facts(database, episode_id, command.idempotency_key)
        assert first["snapshots"] == 1
        assert first["tasks"] == first_task_count
        assert bool(first["initial_task_id"]) is first_bound
        assert first["idempotency_state"] == first_state

        results = await asyncio.gather(*(coordinator.execute(command) for _ in range(10)))
        assert len({item.task_id for item in results}) == 1
        assert all(item.status == "queued" for item in results)

        facts = await render_facts(database, episode_id, command.idempotency_key)
        assert tuple(
            facts[name]
            for name in ("snapshots", "submissions", "tasks", "attempts", "events", "jobs")
        ) == (1, 1, 1, 1, 1, 1)
        assert facts["initial_task_id"] == results[0].task_id
        assert facts["idempotency_state"] == "completed"
        assert facts["response_task_id"] == str(results[0].task_id)

        deliveries = await asyncio.gather(
            *(StartRenderDeliveryHandler(database).execute(results[0].task_id) for _ in range(10))
        )
        assert len({item.id for item in deliveries}) == 1
        assert deliveries[0].status == "rendering"
        assert deliveries[0].render_snapshot_id == facts["render_snapshot_id"]
        assert await delivery_count(database, episode_id) == 1
    finally:
        await database.close()


async def render_facts(
    database: DatabasePool, episode_id: UUID, idempotency_key: str
) -> dict[str, object]:
    scope = f"renderEpisode/{episode_id}"
    async with database.transaction() as connection:
        row = await connection.fetchrow(
            """
            SELECT snapshot.id render_snapshot_id,snapshot.initial_task_id,
                   idem.state idempotency_state,idem.response_ref_json,
                   (SELECT count(*) FROM render_snapshots WHERE episode_id=$1) snapshots,
                   (SELECT count(*) FROM submission_snapshots WHERE episode_id=$1
                       AND type='render_episode') submissions,
                   (SELECT count(*) FROM production_tasks WHERE episode_id=$1
                       AND type='render_episode') tasks,
                   (SELECT count(*) FROM production_attempts attempt JOIN production_tasks task
                       ON task.id=attempt.task_id WHERE task.episode_id=$1
                       AND task.type='render_episode') attempts,
                   (SELECT count(*) FROM task_events event JOIN production_tasks task
                       ON task.id=event.task_id WHERE task.episode_id=$1
                       AND task.type='render_episode') events,
                   (SELECT count(*) FROM task_jobs job JOIN production_tasks task
                       ON task.id=job.task_id WHERE task.episode_id=$1
                       AND task.type='render_episode') jobs
            FROM render_snapshots snapshot
            JOIN idempotency_records idem
              ON idem.operation_scope=$2 AND idem.idempotency_key=$3
            WHERE snapshot.episode_id=$1
            """,
            episode_id,
            scope,
            idempotency_key,
        )
    assert row is not None
    facts = dict(row)
    reference = facts.pop("response_ref_json")
    if isinstance(reference, str):
        reference = json.loads(reference)
    facts["response_task_id"] = reference.get("task_id") if reference else None
    return facts


async def delivery_count(database: DatabasePool, episode_id: UUID) -> int:
    async with database.transaction() as connection:
        value = await connection.fetchval(
            "SELECT count(*) FROM delivery_versions WHERE episode_id=$1", episode_id
        )
    return int(value)
