from __future__ import annotations

from uuid import UUID, uuid4

import pytest

from db.pool import DatabasePool
from repositories.task_outputs import (
    OutputSlotConflict,
    TaskOutputStore,
)
from schemas.jobs import InvalidJobPayload, JobPayload
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.task_submission import SubmitTaskCommand, TaskSubmitter
from workers.dispatch import JobContext, JobHandlerRegistry, UnknownJobHandler


class RecordingHandler:
    def __init__(self) -> None:
        self.contexts: list[JobContext] = []

    async def handle(self, context: JobContext) -> None:
        self.contexts.append(context)


def payload(**overrides: object) -> dict[str, object]:
    value: dict[str, object] = {
        "release_version": "release-1",
        "handler_version": "1",
        "task_id": str(uuid4()),
        "snapshot_id": str(uuid4()),
    }
    value.update(overrides)
    return value


@pytest.mark.parametrize(
    "value",
    [
        {},
        payload(extra="drift"),
        payload(task_id="not-a-uuid"),
        payload(handler_version=""),
    ],
)
def test_job_payload_rejects_missing_unknown_or_invalid_fields(value: dict[str, object]) -> None:
    with pytest.raises(InvalidJobPayload):
        JobPayload.parse(value)


@pytest.mark.asyncio
async def test_registry_dispatches_only_an_exact_compatible_handler() -> None:
    registry = JobHandlerRegistry()
    handler = RecordingHandler()
    registry.register(
        task_type="generate_script",
        release_version="release-1",
        handler_version="1",
        handler=handler,
    )
    parsed = JobPayload.parse(payload())
    context = JobContext(job_id=uuid4(), owner="worker-a", payload=parsed)

    await registry.dispatch("generate_script", context)

    assert handler.contexts == [context]
    with pytest.raises(UnknownJobHandler):
        await registry.dispatch("generate_media", context)
    incompatible = JobContext(
        job_id=uuid4(),
        owner="worker-a",
        payload=JobPayload.parse(payload(release_version="release-2")),
    )
    with pytest.raises(UnknownJobHandler):
        await registry.dispatch("generate_script", incompatible)


async def create_task(database: DatabasePool) -> UUID:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="输出测试", idempotency_key="output:project:01")
    )
    episode_id = project.episode.id
    accepted = await TaskSubmitter(database, release_version="release-1").submit(
        SubmitTaskCommand(
            episode_id=episode_id,
            task_type="generate_script",
            capability="text",
            scope={"episode_id": str(episode_id)},
            input_refs={},
            prompt="生成剧本",
            parameters={"temperature": 0},
            model_profile_id="mock-text-v1",
            provider_id="mock",
            model_id="deterministic-text",
            route_version="text-route-v1",
            schema_version="script-v1",
            operation_scope=f"generateScript/{episode_id}",
            idempotency_key="output:submit:001",
            handler_version="1",
        )
    )
    return accepted.task_id


@pytest.mark.asyncio
async def test_task_output_replay_returns_existing_slot_and_conflict_is_explicit(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        task_id = await create_task(database)
        output_id = uuid4()
        store = TaskOutputStore(database)

        first = await store.record(task_id, "script_version", output_id, ordinal=0)
        replay = await store.record(task_id, "script_version", output_id, ordinal=0)

        assert first == replay
        with pytest.raises(OutputSlotConflict):
            await store.record(task_id, "script_version", uuid4(), ordinal=0)
        async with database.transaction() as connection:
            assert await connection.fetchval("SELECT count(*) FROM task_outputs") == 1
    finally:
        await database.close()
