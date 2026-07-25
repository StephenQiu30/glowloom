from __future__ import annotations

import io
import json
import logging

import pytest

from lanverse.jobs.capacity import WorkerCapacity
from lanverse.jobs.observability import JobLogger, StructuredJsonFormatter
from lanverse.shared_kernel.config import ApplicationSettings


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


def test_job_logs_are_structured_and_reject_sensitive_or_unreviewed_fields() -> None:
    stream = io.StringIO()
    handler = logging.StreamHandler(stream)
    handler.setFormatter(StructuredJsonFormatter())
    logger = logging.getLogger("lanverse.test.jobs")
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
