from __future__ import annotations

from uuid import UUID

import pytest
from fastapi import FastAPI
from httpx import ASGITransport, AsyncClient

from core.config import ApplicationSettings
from db.pool import DatabasePool
from main import create_app
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.task_submission import SubmitTaskCommand, TaskSubmitter


async def submit_task(database_url: str, key: str) -> tuple[UUID, UUID]:
    database = DatabasePool(database_url, min_size=1, max_size=2)
    await database.start()
    try:
        project = await CreateProjectHandler(database).execute(
            CreateProjectCommand(title="任务 API", idempotency_key=f"project:{key}")
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
                idempotency_key=f"submit:{key}",
                handler_version="1",
            )
        )
        return episode_id, accepted.task_id
    finally:
        await database.close()


def app_for(database_url: str) -> FastAPI:
    settings = ApplicationSettings.model_validate(
        {"DATABASE_URL": database_url, "environment": "test"}
    )
    return create_app(settings)


@pytest.mark.asyncio
async def test_task_list_get_and_queued_cancel_are_authoritative(
    migrated_database_url: str,
) -> None:
    episode_id, task_id = await submit_task(migrated_database_url, "task-api-001")
    app = app_for(migrated_database_url)
    async with app.router.lifespan_context(app), AsyncClient(
        transport=ASGITransport(app=app), base_url="http://test"
    ) as client:
        listed = await client.get("/v1/tasks", params={"episode_id": str(episode_id)})
        fetched = await client.get(f"/v1/tasks/{task_id}")
        cancelled = await client.post(
            f"/v1/tasks/{task_id}:cancel",
            headers={
                "Idempotency-Key": "cancel:task:0001",
                "If-Match": fetched.headers["etag"],
            },
        )
        replayed = await client.post(
            f"/v1/tasks/{task_id}:cancel",
            headers={"Idempotency-Key": "cancel:task:0001", "If-Match": fetched.headers["etag"]},
        )

    assert listed.json()["items"][0]["id"] == str(task_id)
    assert fetched.status_code == 200
    assert fetched.headers["etag"] == '"1"'
    assert fetched.json()["poll_after_ms"] == 2000
    assert fetched.json()["current_attempt_id"]
    assert cancelled.status_code == 200
    assert cancelled.headers["etag"] == '"2"'
    assert cancelled.json()["status"] == "cancelled"
    assert replayed.json() == cancelled.json()


@pytest.mark.asyncio
async def test_failed_task_retry_creates_one_new_linked_task(
    migrated_database_url: str,
) -> None:
    _, task_id = await submit_task(migrated_database_url, "task-api-002")
    database = DatabasePool(migrated_database_url, min_size=1, max_size=1)
    await database.start()
    try:
        async with database.transaction() as connection:
            await connection.execute(
                """
                UPDATE production_tasks SET status='failed', error_code='MOCK_FAILED',
                    error_json='{"retryable":true}', next_action='retry',
                    resource_version=2, updated_at=now(), finished_at=now() WHERE id=$1
                """,
                task_id,
            )
            await connection.execute(
                """
                UPDATE production_attempts SET status='failed', error_code='MOCK_FAILED',
                    error_summary='mock failure', finished_at=now() WHERE task_id=$1
                """,
                task_id,
            )
            await connection.execute(
                """
                UPDATE task_jobs SET state='failed', last_error_code='MOCK_FAILED',
                    updated_at=now() WHERE task_id=$1
                """,
                task_id,
            )
    finally:
        await database.close()
    app = app_for(migrated_database_url)
    async with app.router.lifespan_context(app), AsyncClient(
        transport=ASGITransport(app=app), base_url="http://test"
    ) as client:
        retried = await client.post(
            f"/v1/tasks/{task_id}:retry",
            headers={"Idempotency-Key": "retry:task:00001", "If-Match": '"2"'},
        )
        replayed = await client.post(
            f"/v1/tasks/{task_id}:retry",
            headers={"Idempotency-Key": "retry:task:00001", "If-Match": '"2"'},
        )

    assert retried.status_code == 202
    assert retried.json() == replayed.json()
    assert retried.json()["task_id"] != str(task_id)
    database = DatabasePool(migrated_database_url, min_size=1, max_size=1)
    await database.start()
    try:
        async with database.transaction() as connection:
            retry_of = await connection.fetchval(
                "SELECT retry_of_task_id FROM production_tasks WHERE id=$1",
                UUID(retried.json()["task_id"]),
            )
            assert retry_of == task_id
            assert await connection.fetchval("SELECT count(*) FROM production_tasks") == 2
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_task_mutations_reject_stale_etag_and_invalid_retry_state(
    migrated_database_url: str,
) -> None:
    _, task_id = await submit_task(migrated_database_url, "task-api-003")
    app = app_for(migrated_database_url)
    async with app.router.lifespan_context(app), AsyncClient(
        transport=ASGITransport(app=app), base_url="http://test"
    ) as client:
        stale = await client.post(
            f"/v1/tasks/{task_id}:cancel",
            headers={"Idempotency-Key": "cancel:stale:001", "If-Match": '"9"'},
        )
        invalid_retry = await client.post(
            f"/v1/tasks/{task_id}:retry",
            headers={"Idempotency-Key": "retry:queued:001", "If-Match": '"1"'},
        )
        missing = await client.get("/v1/tasks/00000000-0000-0000-0000-000000000000")

    assert stale.status_code == 412
    assert stale.json()["code"] == "VERSION_CONFLICT"
    assert invalid_retry.status_code == 409
    assert invalid_retry.json()["code"] == "TASK_NOT_RETRYABLE"
    assert missing.status_code == 404


def test_task_openapi_operations_extend_the_single_contract() -> None:
    schema = create_app().openapi()
    operations = {
        operation["operationId"]
        for path in schema["paths"].values()
        for operation in path.values()
        if isinstance(operation, dict) and "operationId" in operation
    }
    assert {"listTasks", "getTask", "cancelTask", "retryTask"} <= operations
