from __future__ import annotations

from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest

from db.pool import DatabasePool
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.task_attempts import (
    AutomaticRetryExhausted,
    AutomaticRetryHandler,
)
from services.task_submission import SubmitTaskCommand, TaskSubmitter
from services.tasks import (
    CancelTaskHandler,
    TaskQueryService,
)
from workers.dispatch import JobHandlerRegistry
from workers.provider_execution import (
    FaultInjector,
    InjectedFault,
    ProviderExecutionHandler,
    ProviderOutcome,
)
from workers.runner import TaskJobRunner

NOW = datetime(2030, 7, 25, 12, 0, tzinfo=UTC)


class FailOnce(FaultInjector):
    def __init__(self, point: str) -> None:
        self.point = point
        self.triggered = False

    def hit(self, point: str) -> None:
        if point == self.point and not self.triggered:
            self.triggered = True
            raise InjectedFault(point)


class LateSuccessProvider:
    def __init__(self) -> None:
        self.requests: dict[str, str] = {}
        self.submit_calls = 0

    async def submit(self, request_key: str, prompt: str) -> ProviderOutcome:
        self.submit_calls += 1
        self.requests[request_key] = "provider-late"
        return ProviderOutcome(state="succeeded", provider_request_id="provider-late")

    async def reconcile(self, request_key: str) -> ProviderOutcome:
        request_id = self.requests.get(request_key)
        if request_id is None:
            return ProviderOutcome(state="not_found", provider_request_id=None)
        return ProviderOutcome(state="succeeded", provider_request_id=request_id)

    async def download(self, outcome: ProviderOutcome) -> None:
        assert outcome.provider_request_id == "provider-late"


async def submit_task(database: DatabasePool, key: str) -> UUID:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="重试取消", idempotency_key=f"policy:project:{key}")
    )
    episode_id = project.episode.id
    accepted = await TaskSubmitter(database, release_version="test-release").submit(
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
            idempotency_key=f"policy:submit:{key}",
            handler_version="1",
        )
    )
    return accepted.task_id


@pytest.mark.asyncio
async def test_automatic_retry_adds_parented_attempts_up_to_three_without_duplicates(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        task_id = await submit_task(database, "attempts")
        retry = AutomaticRetryHandler(database)
        async with database.transaction() as connection:
            await connection.execute(
                """
                UPDATE production_tasks SET status='running', resource_version=2,
                    progress_json='{"phase":"provider","completed":0,"total":1}',
                    updated_at=now() WHERE id=$1
                """,
                task_id,
            )
            await connection.execute(
                """
                INSERT INTO task_events(
                    event_id,task_id,task_resource_version,event_type,correlation_id,data_json
                ) VALUES($1,$2,2,'task.started',$3,'{}')
                """,
                uuid4(),
                task_id,
                uuid4(),
            )
            await connection.execute(
                """
                UPDATE production_attempts SET status='failed', error_code='TEMPORARY',
                    error_summary='temporary', finished_at=now() WHERE task_id=$1
                """,
                task_id,
            )
        second = await retry.execute(task_id)
        replay = await retry.execute(task_id)
        assert second == replay
        assert second.attempt_no == 2
        async with database.transaction() as connection:
            await connection.execute(
                """
                UPDATE production_attempts SET status='failed', error_code='TEMPORARY',
                    error_summary='temporary', finished_at=now()
                WHERE task_id=$1 AND attempt_no=2
                """,
                task_id,
            )
        third = await retry.execute(task_id)
        assert third.attempt_no == 3
        async with database.transaction() as connection:
            await connection.execute(
                """
                UPDATE production_attempts SET status='failed', error_code='TEMPORARY',
                    error_summary='temporary', finished_at=now()
                WHERE task_id=$1 AND attempt_no=3
                """,
                task_id,
            )
        with pytest.raises(AutomaticRetryExhausted):
            await retry.execute(task_id)
        async with database.transaction() as connection:
            assert await connection.fetchval(
                "SELECT count(*) FROM production_attempts WHERE task_id=$1", task_id
            ) == 3
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_provider_success_after_cancel_request_cannot_make_task_succeeded(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id = await submit_task(database, "late")
        provider = LateSuccessProvider()
        fault = FailOnce("after_provider_accept")
        registry = JobHandlerRegistry()
        registry.register(
            task_type="generate_script",
            release_version="test-release",
            handler_version="1",
            handler=ProviderExecutionHandler(database, provider=provider, fault=fault),
        )
        first_runner = TaskJobRunner(
            database,
            registry=registry,
            owner="worker-a",
            lease_duration=timedelta(seconds=10),
            fault=fault,
        )
        with pytest.raises(InjectedFault):
            await first_runner.run_once(now=NOW)
        running = await TaskQueryService(database).get(task_id)
        assert running.status == "running"
        await CancelTaskHandler(database).execute(task_id, 2, "cancel:late:0001")

        recovered = TaskJobRunner(
            database,
            registry=registry,
            owner="worker-b",
            lease_duration=timedelta(seconds=10),
            fault=fault,
        )
        await recovered.run_once(now=NOW + timedelta(seconds=11))
        task = await TaskQueryService(database).get(task_id)

        assert task.status == "cancelled"
        assert provider.submit_calls == 1
    finally:
        await database.close()
