from __future__ import annotations

from datetime import UTC, datetime, timedelta
from uuid import UUID

import pytest

from db.pool import DatabasePool
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.task_submission import SubmitTaskCommand, TaskSubmitter
from services.tasks import TaskQueryService
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


class RecoverableProviderFake:
    def __init__(self, *, reconcile_unknown: bool = False) -> None:
        self.reconcile_unknown = reconcile_unknown
        self.requests: dict[str, str] = {}
        self.submit_calls = 0
        self.reconcile_calls = 0
        self.download_calls = 0

    async def submit(self, request_key: str, prompt: str) -> ProviderOutcome:
        assert prompt == "生成剧本"
        self.submit_calls += 1
        request_id = f"provider-{self.submit_calls}"
        self.requests[request_key] = request_id
        return ProviderOutcome(state="succeeded", provider_request_id=request_id)

    async def reconcile(self, request_key: str) -> ProviderOutcome:
        self.reconcile_calls += 1
        if self.reconcile_unknown:
            return ProviderOutcome(state="unknown", provider_request_id=None)
        request_id = self.requests.get(request_key)
        if request_id is None:
            return ProviderOutcome(state="not_found", provider_request_id=None)
        return ProviderOutcome(state="succeeded", provider_request_id=request_id)

    async def download(self, outcome: ProviderOutcome) -> None:
        assert outcome.provider_request_id
        self.download_calls += 1


async def submit_task(database: DatabasePool, key: str) -> UUID:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="恢复测试", idempotency_key=f"recover:project:{key}")
    )
    episode_id = project.episode.id
    result = await TaskSubmitter(database, release_version="test-release").submit(
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
            idempotency_key=f"recover:submit:{key}",
            handler_version="1",
        )
    )
    return result.task_id


def runner(
    database: DatabasePool,
    provider: RecoverableProviderFake,
    fault: FaultInjector,
    owner: str,
) -> TaskJobRunner:
    registry = JobHandlerRegistry()
    registry.register(
        task_type="generate_script",
        release_version="test-release",
        handler_version="1",
        handler=ProviderExecutionHandler(database, provider=provider, fault=fault),
    )
    return TaskJobRunner(
        database,
        registry=registry,
        owner=owner,
        lease_duration=timedelta(seconds=10),
        fault=fault,
    )


@pytest.mark.asyncio
@pytest.mark.parametrize(
    "fault_point",
    [
        "after_claim",
        "before_provider_submit",
        "after_provider_accept",
        "before_download",
        "after_download",
        "before_registration",
    ],
)
async def test_expired_lease_recovery_reconciles_without_duplicate_provider_submit(
    migrated_database_url: str, fault_point: str
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id = await submit_task(database, fault_point)
        provider = RecoverableProviderFake()
        fault = FailOnce(fault_point)
        with pytest.raises(InjectedFault):
            await runner(database, provider, fault, "worker-a").run_once(now=NOW)

        processed = await runner(database, provider, fault, "worker-b").run_once(
            now=NOW + timedelta(seconds=11)
        )
        task = await TaskQueryService(database).get(task_id)

        assert processed is True
        assert task.status == "succeeded"
        assert provider.submit_calls == 1
        assert provider.reconcile_calls <= 1
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_unprovable_provider_acceptance_converges_to_explicit_unknown(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id = await submit_task(database, "unknown")
        provider = RecoverableProviderFake(reconcile_unknown=True)
        fault = FailOnce("after_provider_accept")
        with pytest.raises(InjectedFault):
            await runner(database, provider, fault, "worker-a").run_once(now=NOW)
        await runner(database, provider, fault, "worker-b").run_once(
            now=NOW + timedelta(seconds=11)
        )
        task = await TaskQueryService(database).get(task_id)

        assert task.status == "unknown"
        assert task.error_code == "PROVIDER_ACCEPTANCE_UNKNOWN"
        assert provider.submit_calls == 1
    finally:
        await database.close()
