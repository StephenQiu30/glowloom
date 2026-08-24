import asyncio
import hashlib
import logging
from datetime import UTC, datetime, timedelta
from typing import Any
from uuid import UUID

import httpx
import pytest
from prometheus_client import REGISTRY
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.integrations.minio import MinioObjectStorage
from app.modules.governance.audit.models import AuditEvent
from app.modules.messaging.models import OutboxEvent
from app.modules.production.models import Task
from app.modules.scheduling import repository, service
from app.modules.scheduling.dispatcher import dispatch_due_schedules
from app.modules.scheduling.models import Schedule, ScheduleFire
from tests.support.identity_builders import register_identity_response


def _metric_value(name: str, labels: dict[str, str]) -> float:
    return REGISTRY.get_sample_value(name, labels) or 0


async def _identity(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> tuple[dict[str, str], UUID]:
    response = await register_identity_response(client, email=email)
    assert response.status_code == 201
    data = response.json()["data"]
    return (
        {"authorization": f"Bearer {data['access_token']}"},
        UUID(data["workspace"]["id"]),
    )


async def _create_cleanup_schedule(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    workspace_id: UUID,
    monkeypatch: pytest.MonkeyPatch,
    *,
    suffix: str,
) -> UUID:
    async def presign_upload(_: MinioObjectStorage, object_key: str, expires_seconds: int) -> str:
        return f"https://storage.invalid/{object_key}?expires={expires_seconds}"

    monkeypatch.setattr(MinioObjectStorage, "presign_upload", presign_upload)
    content = f"scheduler-{suffix}".encode()
    response = await client.post(
        "/api/v1/media/uploads",
        headers=headers,
        json={
            "workspace_id": str(workspace_id),
            "kind": "image",
            "filename": f"{suffix}.png",
            "size_bytes": len(content),
            "mime_type": "image/png",
            "sha256": hashlib.sha256(content).hexdigest(),
            "idempotency_key": f"scheduler-{suffix}",
        },
    )
    assert response.status_code == 201
    return workspace_id


async def _cleanup_schedule(
    session_factory: async_sessionmaker[AsyncSession],
    workspace_id: UUID,
) -> Schedule:
    async with session_factory() as session:
        schedule = await session.scalar(
            select(Schedule).where(
                Schedule.workspace_id == workspace_id,
                Schedule.handler_name == "cleanup_expired_uploads",
            )
        )
        assert schedule is not None
        return schedule


@pytest.mark.asyncio
async def test_owner_configures_registered_cleanup_cron_without_exposing_payload(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    headers, workspace_id = await _identity(client, email="scheduler-configuration@example.com")
    other_headers, _ = await _identity(client, email="scheduler-configuration-other@example.com")
    await _create_cleanup_schedule(
        client, headers, workspace_id, monkeypatch, suffix="configuration"
    )
    schedule = await _cleanup_schedule(session_factory, workspace_id)
    effective_from = datetime(2026, 10, 31, 5, 31, tzinfo=UTC)
    payload = {
        "expected_revision": schedule.revision,
        "effective_from": effective_from.isoformat(),
        "kind": "cron",
        "cron_expression": "30 1 * * *",
        "timezone": "America/New_York",
        "misfire_policy": "catch_up",
        "max_catch_up": 2,
        "misfire_grace_seconds": 30,
    }

    configured = await client.put(
        f"/api/v1/schedules/{schedule.id}/configuration",
        headers=headers,
        json=payload,
    )
    assert configured.status_code == 200
    data = configured.json()["data"]
    assert data["kind"] == "cron"
    assert data["timezone"] == "America/New_York"
    assert data["misfire_policy"] == "catch_up"
    assert data["max_catch_up"] == 2
    assert data["rule"] == {
        "kind": "cron",
        "expression": "30 1 * * *",
        "misfire_grace_seconds": 30,
    }
    assert data["next_fire_at"] == "2026-11-01T05:30:00Z"
    assert "payload" not in data

    hidden = await client.put(
        f"/api/v1/schedules/{schedule.id}/configuration",
        headers=other_headers,
        json=payload,
    )
    assert hidden.status_code == 404

    invalid = await client.put(
        f"/api/v1/schedules/{schedule.id}/configuration",
        headers=headers,
        json={
            **payload,
            "expected_revision": data["revision"],
            "cron_expression": "@daily",
        },
    )
    assert invalid.status_code == 422

    async with session_factory() as session:
        stored = await session.get(Schedule, schedule.id)
        audits = list(
            await session.scalars(
                select(AuditEvent).where(
                    AuditEvent.target_id == schedule.id,
                    AuditEvent.action == "schedule.configured",
                )
            )
        )
        assert stored is not None
        assert stored.payload == {"workspace_id": str(workspace_id)}
        assert stored.rule == {
            "expression": "30 1 * * *",
            "misfire_grace_seconds": 30,
        }
        assert len(audits) == 1


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("policy", "max_catch_up", "expected_fires"),
    [("skip", 0, 0), ("run_once", 0, 1), ("catch_up", 3, 3)],
)
async def test_misfire_policies_have_bounded_postgresql_concurrency_evidence(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
    policy: str,
    max_catch_up: int,
    expected_fires: int,
) -> None:
    headers, workspace_id = await _identity(client, email=f"scheduler-policy-{policy}@example.com")
    await _create_cleanup_schedule(
        client, headers, workspace_id, monkeypatch, suffix=f"policy-{policy}"
    )
    schedule = await _cleanup_schedule(session_factory, workspace_id)
    now = datetime.now(UTC)
    async with session_factory() as session:
        async with session.begin():
            stored = await session.get(Schedule, schedule.id, with_for_update=True)
            assert stored is not None
            stored.next_fire_at = now - timedelta(minutes=5, seconds=45)
            stored.rule = {"seconds": 60, "misfire_grace_seconds": 30}
            stored.misfire_policy = policy
            stored.max_catch_up = max_catch_up

    results = await asyncio.gather(
        dispatch_due_schedules(
            session_factory,
            dispatcher_id=f"{policy}-dispatcher-a",
            now=now,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        ),
        dispatch_due_schedules(
            session_factory,
            dispatcher_id=f"{policy}-dispatcher-b",
            now=now,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        ),
    )
    assert sum(results) == expected_fires

    async with session_factory() as session:
        stored = await session.get(Schedule, schedule.id)
        assert stored is not None and stored.next_fire_at is not None
        assert stored.next_fire_at > now
        assert (
            await session.scalar(
                select(func.count())
                .select_from(ScheduleFire)
                .where(ScheduleFire.schedule_id == schedule.id)
            )
            == expected_fires
        )
        assert (
            await session.scalar(
                select(func.count()).select_from(Task).where(Task.request_id == workspace_id)
            )
            == expected_fires
        )
        assert (
            await session.scalar(
                select(func.count())
                .select_from(OutboxEvent)
                .where(OutboxEvent.workspace_id == workspace_id)
            )
            == expected_fires
        )


@pytest.mark.asyncio
async def test_expired_lease_and_task_boundary_rollback_recover_without_duplicates(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    headers, workspace_id = await _identity(client, email="scheduler-boundary-recovery@example.com")
    await _create_cleanup_schedule(
        client, headers, workspace_id, monkeypatch, suffix="boundary-recovery"
    )
    schedule = await _cleanup_schedule(session_factory, workspace_id)
    now = datetime.now(UTC)
    async with session_factory() as session:
        async with session.begin():
            stored = await session.get(Schedule, schedule.id, with_for_update=True)
            assert stored is not None
            stored.next_fire_at = now - timedelta(seconds=1)

    async with session_factory() as session:
        async with session.begin():
            claimed = await repository.claim_due_schedules(
                session,
                dispatcher_id="crashed-dispatcher",
                now=now,
                batch_size=10,
                lease_duration=timedelta(seconds=30),
            )
            assert claimed == [schedule.id]
    async with session_factory() as session:
        async with session.begin():
            assert (
                await repository.claim_due_schedules(
                    session,
                    dispatcher_id="early-recovery",
                    now=now + timedelta(seconds=29),
                    batch_size=10,
                    lease_duration=timedelta(seconds=30),
                )
                == []
            )

    real_factory = service.create_upload_cleanup_task

    class BoundaryCrash(RuntimeError):
        pass

    async def create_then_crash(*args: Any, **kwargs: Any) -> Any:
        await real_factory(*args, **kwargs)
        raise BoundaryCrash("must not be persisted")

    with monkeypatch.context() as context:
        context.setattr(service, "create_upload_cleanup_task", create_then_crash)
        assert (
            await dispatch_due_schedules(
                session_factory,
                dispatcher_id="recovered-dispatcher",
                now=now + timedelta(seconds=31),
                batch_size=10,
                lease_duration=timedelta(seconds=30),
            )
            == 0
        )

    async with session_factory() as session:
        stored = await session.get(Schedule, schedule.id)
        assert stored is not None
        assert stored.failure_count == 1
        assert stored.last_error == "BoundaryCrash"
        assert stored.next_attempt_at is not None
        retry_at = stored.next_attempt_at + timedelta(microseconds=1)
        assert await session.scalar(select(func.count()).select_from(ScheduleFire)) == 0
        assert await session.scalar(select(func.count()).select_from(Task)) == 0
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 0

    assert (
        await dispatch_due_schedules(
            session_factory,
            dispatcher_id="retry-dispatcher",
            now=retry_at,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        )
        == 1
    )
    assert (
        await dispatch_due_schedules(
            session_factory,
            dispatcher_id="restart-dispatcher",
            now=retry_at,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        )
        == 0
    )

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScheduleFire)) == 1
        assert await session.scalar(select(func.count()).select_from(Task)) == 1
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 1


@pytest.mark.asyncio
async def test_failure_limit_and_unregistered_handler_stop_automatic_dispatch(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
) -> None:
    headers, workspace_id = await _identity(client, email="scheduler-manual-attention@example.com")
    await _create_cleanup_schedule(
        client, headers, workspace_id, monkeypatch, suffix="manual-attention"
    )
    schedule = await _cleanup_schedule(session_factory, workspace_id)
    now = datetime.now(UTC)
    caplog.set_level(logging.ERROR, logger="lanverse.scheduler")
    failed_before = _metric_value(
        "lanverse_schedule_dispatch_total",
        {"handler": "cleanup_expired_uploads", "result": "failed"},
    )
    exhausted_before = _metric_value(
        "lanverse_schedule_manual_attention_total",
        {"handler": "cleanup_expired_uploads", "reason": "retry_exhausted"},
    )

    class TemporaryDispatchFailure(RuntimeError):
        pass

    async def always_fail(*_: Any, **__: Any) -> Any:
        raise TemporaryDispatchFailure("dependency detail must not be stored")

    with monkeypatch.context() as context:
        context.setattr(service, "create_upload_cleanup_task", always_fail)
        attempt_at = now
        for expected_count in range(1, 6):
            async with session_factory() as session:
                async with session.begin():
                    stored = await session.get(Schedule, schedule.id, with_for_update=True)
                    assert stored is not None
                    if expected_count == 1:
                        stored.next_fire_at = attempt_at - timedelta(seconds=1)
            assert (
                await dispatch_due_schedules(
                    session_factory,
                    dispatcher_id=f"failing-dispatcher-{expected_count}",
                    now=attempt_at,
                    batch_size=10,
                    lease_duration=timedelta(seconds=30),
                )
                == 0
            )
            async with session_factory() as session:
                stored = await session.get(Schedule, schedule.id)
                assert stored is not None and stored.failure_count == expected_count
                if expected_count < 5:
                    assert stored.status == "active"
                    assert stored.next_attempt_at is not None
                    attempt_at = stored.next_attempt_at + timedelta(microseconds=1)

    async with session_factory() as session:
        stored = await session.get(Schedule, schedule.id)
        assert stored is not None
        assert stored.status == "manual_attention"
        assert stored.last_error == "TemporaryDispatchFailure"
        assert stored.next_attempt_at is None
        assert "dependency detail" not in stored.last_error
    assert (
        _metric_value(
            "lanverse_schedule_dispatch_total",
            {"handler": "cleanup_expired_uploads", "result": "failed"},
        )
        == failed_before + 5
    )
    assert (
        _metric_value(
            "lanverse_schedule_manual_attention_total",
            {"handler": "cleanup_expired_uploads", "reason": "retry_exhausted"},
        )
        == exhausted_before + 1
    )
    retry_records = [
        record
        for record in caplog.records
        if getattr(record, "schedule_id", None) == str(schedule.id)
    ]
    assert len(retry_records) == 5
    assert retry_records[-1].getMessage() == ("schedule dispatch stopped for operator action")
    assert all(
        getattr(record, "error_type", None) == "TemporaryDispatchFailure"
        for record in retry_records
    )
    assert all("dependency detail" not in record.getMessage() for record in retry_records)

    async with session_factory() as session:
        async with session.begin():
            stored = await session.get(Schedule, schedule.id, with_for_update=True)
            assert stored is not None
            stored.status = "active"
            stored.handler_name = "os.system"
            stored.failure_count = 0
            stored.next_attempt_at = None
            stored.next_fire_at = attempt_at - timedelta(seconds=1)

    assert (
        await dispatch_due_schedules(
            session_factory,
            dispatcher_id="unregistered-handler-dispatcher",
            now=attempt_at,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        )
        == 0
    )
    async with session_factory() as session:
        stored = await session.get(Schedule, schedule.id)
        assert stored is not None
        assert stored.status == "manual_attention"
        assert stored.failure_count == 1
        assert stored.last_error == "UnregisteredScheduleHandler"
        assert stored.next_attempt_at is None

    listed = await client.get(
        "/api/v1/schedules",
        headers=headers,
        params={"workspace_id": str(workspace_id)},
    )
    assert listed.status_code == 200
    public = next(item for item in listed.json()["data"]["items"] if item["id"] == str(schedule.id))
    assert public["handler_name"] == "unregistered"

    invalid_headers, invalid_workspace_id = await _identity(
        client, email="scheduler-invalid-payload@example.com"
    )
    await _create_cleanup_schedule(
        client,
        invalid_headers,
        invalid_workspace_id,
        monkeypatch,
        suffix="invalid-payload",
    )
    invalid_schedule = await _cleanup_schedule(session_factory, invalid_workspace_id)
    configuration_before = _metric_value(
        "lanverse_schedule_manual_attention_total",
        {"handler": "cleanup_expired_uploads", "reason": "configuration"},
    )
    async with session_factory() as session:
        async with session.begin():
            stored = await session.get(Schedule, invalid_schedule.id, with_for_update=True)
            assert stored is not None
            stored.payload = {
                "workspace_id": str(invalid_workspace_id),
                "unexpected": "must be rejected",
            }
            stored.next_fire_at = attempt_at - timedelta(seconds=1)

    assert (
        await dispatch_due_schedules(
            session_factory,
            dispatcher_id="invalid-payload-dispatcher",
            now=attempt_at,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        )
        == 0
    )
    async with session_factory() as session:
        stored = await session.get(Schedule, invalid_schedule.id)
        assert stored is not None
        assert stored.status == "manual_attention"
        assert stored.failure_count == 1
        assert stored.last_error == "InvalidSchedulePayload"
        assert stored.next_attempt_at is None
    assert (
        _metric_value(
            "lanverse_schedule_manual_attention_total",
            {"handler": "cleanup_expired_uploads", "reason": "configuration"},
        )
        == configuration_before + 1
    )


@pytest.mark.asyncio
async def test_dispatch_can_use_postgresql_clock_as_the_authority(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    headers, workspace_id = await _identity(client, email="scheduler-database-clock@example.com")
    await _create_cleanup_schedule(
        client, headers, workspace_id, monkeypatch, suffix="database-clock"
    )
    schedule = await _cleanup_schedule(session_factory, workspace_id)
    async with session_factory() as session:
        async with session.begin():
            database_now = await session.scalar(select(func.current_timestamp()))
            assert database_now is not None
            stored = await session.get(Schedule, schedule.id, with_for_update=True)
            assert stored is not None
            stored.next_fire_at = database_now - timedelta(seconds=1)

    assert (
        await dispatch_due_schedules(
            session_factory,
            dispatcher_id="database-clock-dispatcher",
            now=None,
            batch_size=10,
            lease_duration=timedelta(seconds=30),
        )
        == 1
    )
