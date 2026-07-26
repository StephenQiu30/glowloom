from __future__ import annotations

import io
import json
import logging
from datetime import UTC, datetime, timedelta

import pytest

from core.config import ApplicationSettings
from db.pool import DatabasePool
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.task_submission import SubmitTaskCommand, TaskSubmitter
from workers.capacity import WorkerCapacity
from workers.dispatch import JobHandlerRegistry
from workers.observability import JobLogger, StructuredJsonFormatter
from workers.provider_execution import FaultInjector
from workers.runner import TaskJobRunner


def test_provider_and_worker_resource_limits_are_explicit_and_bounded() -> None:
    settings = ApplicationSettings()

    assert settings.provider_max_concurrency == 3
    assert settings.provider_submit_timeout_seconds == 30
    assert settings.provider_status_timeout_seconds == 10
    assert settings.provider_poll_min_seconds == 2
    assert settings.provider_poll_max_seconds == 10
    assert settings.text_task_timeout_seconds == 120
    assert settings.video_task_timeout_seconds == 600
    assert settings.text_status_poll_limit == 60
    assert settings.video_status_poll_limit == 300
    assert settings.worker_lease_seconds == 30
    assert settings.worker_heartbeat_seconds == 10

    with pytest.raises(ValueError):
        ApplicationSettings(provider_max_concurrency=4)
    with pytest.raises(ValueError):
        ApplicationSettings(provider_submit_timeout_seconds=0)


@pytest.mark.asyncio
async def test_capacity_gate_refuses_work_without_waiting_or_overclaiming() -> None:
    capacity = WorkerCapacity(limit=3)

    assert await capacity.try_acquire()
    assert await capacity.try_acquire()
    assert await capacity.try_acquire()
    assert await capacity.try_acquire() is False
    assert capacity.active == 3

    await capacity.release()
    assert await capacity.try_acquire()
    assert capacity.active == 3


@pytest.mark.asyncio
async def test_exhausted_capacity_stops_before_claiming_a_persisted_job(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        project = await CreateProjectHandler(database).execute(
            CreateProjectCommand(title="容量测试", idempotency_key="capacity:project:1")
        )
        episode_id = project.episode.id
        await TaskSubmitter(database, release_version="test-release").submit(
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
                idempotency_key="capacity:submit:01",
                handler_version="1",
            )
        )
        capacity = WorkerCapacity(limit=1)
        assert await capacity.try_acquire()
        runner = TaskJobRunner(
            database,
            registry=JobHandlerRegistry(),
            owner="worker-full",
            lease_duration=timedelta(seconds=10),
            fault=FaultInjector(),
            capacity=capacity,
        )

        assert await runner.run_once(now=datetime(2030, 1, 1, tzinfo=UTC)) is False
        async with database.transaction() as connection:
            row = await connection.fetchrow("SELECT state,attempts FROM task_jobs")
        assert tuple(row) == ("pending", 0)
    finally:
        await database.close()

def test_job_logs_are_structured_and_reject_sensitive_or_unreviewed_fields() -> None:
    stream = io.StringIO()
    handler = logging.StreamHandler(stream)
    handler.setFormatter(StructuredJsonFormatter())
    logger = logging.getLogger("test.jobs")
    logger.handlers = [handler]
    logger.propagate = False
    logger.setLevel(logging.INFO)
    jobs = JobLogger(logger)

    jobs.info(
        "job_claimed",
        release_version="release-1",
        task_id="task-1",
        attempt_id="attempt-1",
        job_id="job-1",
        request_id="request-1",
        error_code=None,
    )
    record = json.loads(stream.getvalue())

    assert record["event"] == "job_claimed"
    assert record["task_id"] == "task-1"
    assert set(record) >= {
        "timestamp",
        "level",
        "logger",
        "event",
        "release_version",
        "request_id",
        "task_id",
        "attempt_id",
        "job_id",
        "error_code",
    }
    with pytest.raises(ValueError):
        jobs.info("unsafe", prompt="full private prompt")
    with pytest.raises(ValueError):
        jobs.info("unsafe", authorization="secret")
    assert "private prompt" not in stream.getvalue()
    assert "secret" not in stream.getvalue()
