from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.governance.audit.models import AuditEvent
from app.modules.production import generation_cancellation
from app.modules.production.models import CostEntry, Reservation, Task
from tests.integration.test_generation_requests_api import (
    create_ready_generation_shot,
    seed_active_capability,
)


async def submit_queued_generation(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    submission_key: str,
) -> tuple[dict[str, str], dict[str, Any], dict[str, Any]]:
    headers, episode, refs, shot_facts = await create_ready_generation_shot(
        client,
        session_factory,
    )
    capability = await seed_active_capability(
        session_factory,
        amount="1.250000",
        high_cost_threshold="10.000000",
    )
    preflight_payload = {
        "workspace_id": str(refs["workspace_id"]),
        "shot_spec_version_id": shot_facts["spec_version"]["id"],
        "capability_id": str(capability.id),
        "parameters": {"resolution": "1080p"},
    }
    preflight = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
        headers=headers,
        json=preflight_payload,
    )
    assert preflight.status_code == 200
    preflight_result = preflight.json()["data"]
    submitted = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
        headers=headers,
        json={
            **preflight_payload,
            "preflight_hash": preflight_result["preflight_hash"],
            "preflight_expires_at": preflight_result["expires_at"],
            "warning_acknowledgements": ["STYLE_REFERENCE_MISSING"],
            "high_cost_confirmed": False,
            "idempotency_key": submission_key,
        },
    )
    assert submitted.status_code == 201
    return headers, episode, submitted.json()["data"]


@pytest.mark.asyncio
async def test_queued_generation_cancel_releases_reservation_once_and_is_replayable(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, submitted = await submit_queued_generation(
        client,
        session_factory,
        submission_key="queued-generation-for-cancellation",
    )
    task = submitted["task"]
    cancellation = await client.post(
        f"/api/v1/tasks/{task['id']}/cancel",
        headers=headers,
        json={
            "workspace_id": task["workspace_id"],
            "expected_revision": task["revision"],
            "idempotency_key": "cancel-before-provider-dispatch",
            "reason": "user_requested",
        },
    )

    assert cancellation.status_code == 200
    data = cancellation.json()["data"]
    assert data["replayed"] is False
    assert data["task"]["status"] == "cancelled"
    assert data["task"]["progress_stage"] == "cancelled_before_dispatch"
    assert data["task"]["cancel_status"] == "accepted"
    assert data["task"]["next_action"] is None
    assert data["task"]["revision"] == 2
    assert data["reservation"]["status"] == "released"
    assert data["reservation"]["reserved_amount"] == "1.250000"
    assert data["reservation"]["revision"] == 2
    assert data["release_cost_entry"]["entry_type"] == "release"
    assert data["release_cost_entry"]["amount"] == "1.250000"

    repeated = await client.post(
        f"/api/v1/tasks/{task['id']}/cancel",
        headers=headers,
        json={
            "workspace_id": task["workspace_id"],
            "expected_revision": task["revision"],
            "idempotency_key": "another-replay-key",
            "reason": "input_changed",
        },
    )
    assert repeated.status_code == 200
    replay = repeated.json()["data"]
    assert replay["replayed"] is True
    assert replay["task"]["id"] == data["task"]["id"]
    assert replay["release_cost_entry"]["id"] == data["release_cost_entry"]["id"]

    costs = await client.get(
        "/api/v1/costs",
        headers=headers,
        params={
            "workspace_id": task["workspace_id"],
            "project_id": episode["project_id"],
        },
    )
    assert costs.status_code == 200
    assert costs.json()["data"]["summary"] == {
        "reserved": "1.250000",
        "settled": "0.000000",
        "released": "1.250000",
        "adjustments": "0.000000",
        "remaining_reserved": "0.000000",
    }

    async with session_factory() as session:
        assert (
            await session.scalar(
                select(func.count())
                .select_from(CostEntry)
                .where(CostEntry.reservation_id == UUID(data["reservation"]["id"]))
            )
            == 2
        )
        audit = await session.scalar(
            select(AuditEvent).where(
                AuditEvent.action == "task.cancelled",
                AuditEvent.target_id == UUID(task["id"]),
            )
        )
        assert audit is not None
        assert audit.event_metadata["reason"] == "user_requested"
        assert "idempotency_key" not in audit.event_metadata
        release = await session.get(
            CostEntry,
            UUID(data["release_cost_entry"]["id"]),
        )
        assert release is not None
        assert "cancel-before-provider-dispatch" not in release.idempotency_key


@pytest.mark.asyncio
async def test_generation_cancel_rejects_stale_or_started_task_without_releasing_cost(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, submitted = await submit_queued_generation(
        client,
        session_factory,
        submission_key="queued-generation-for-conflict",
    )
    task = submitted["task"]

    stale = await client.post(
        f"/api/v1/tasks/{task['id']}/cancel",
        headers=headers,
        json={
            "workspace_id": task["workspace_id"],
            "expected_revision": 999,
            "idempotency_key": "stale-cancel-command",
            "reason": "budget_changed",
        },
    )
    assert stale.status_code == 409
    assert stale.json()["error"]["code"] == "version_conflict"
    assert stale.json()["error"]["next_action"] == "reload_task"

    async with session_factory() as session, session.begin():
        persisted = await session.get(Task, UUID(task["id"]), with_for_update=True)
        assert persisted is not None
        persisted.status = "running"
        persisted.progress_stage = "dispatching"
        persisted.revision = 2

    started = await client.post(
        f"/api/v1/tasks/{task['id']}/cancel",
        headers=headers,
        json={
            "workspace_id": task["workspace_id"],
            "expected_revision": 2,
            "idempotency_key": "started-cancel-command",
            "reason": "user_requested",
        },
    )
    assert started.status_code == 409
    assert started.json()["error"]["code"] == "state_conflict"
    assert (
        started.json()["error"]["next_action"]
        == "wait_for_provider_cancellation_support"
    )

    async with session_factory() as session:
        reservation = await session.scalar(
            select(Reservation).where(
                Reservation.id == UUID(submitted["reservation"]["id"])
            )
        )
        assert reservation is not None
        assert reservation.status == "active"
        assert (
            await session.scalar(
                select(func.count())
                .select_from(CostEntry)
                .where(CostEntry.reservation_id == reservation.id)
            )
            == 1
        )


@pytest.mark.asyncio
async def test_generation_cancel_rolls_back_task_reservation_and_release_when_audit_fails(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    headers, _, submitted = await submit_queued_generation(
        client,
        session_factory,
        submission_key="queued-generation-for-rollback",
    )
    task = submitted["task"]

    def fail_audit(*args: object, **kwargs: object) -> None:
        _ = args, kwargs
        raise RuntimeError("forced cancellation audit failure")

    monkeypatch.setattr(generation_cancellation, "append_audit_event", fail_audit)
    with pytest.raises(RuntimeError, match="forced cancellation audit failure"):
        await client.post(
            f"/api/v1/tasks/{task['id']}/cancel",
            headers=headers,
            json={
                "workspace_id": task["workspace_id"],
                "expected_revision": task["revision"],
                "idempotency_key": "cancel-rollback-command",
                "reason": "user_requested",
            },
        )

    async with session_factory() as session:
        persisted_task = await session.get(Task, UUID(task["id"]))
        reservation = await session.get(
            Reservation,
            UUID(submitted["reservation"]["id"]),
        )
        assert persisted_task is not None
        assert reservation is not None
        assert persisted_task.status == "queued"
        assert persisted_task.cancel_status == "none"
        assert persisted_task.revision == 1
        assert reservation.status == "active"
        assert reservation.revision == 1
        assert (
            await session.scalar(
                select(func.count())
                .select_from(CostEntry)
                .where(CostEntry.reservation_id == reservation.id)
            )
            == 1
        )


@pytest.mark.asyncio
async def test_generation_cancel_is_workspace_isolated(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    _, _, submitted = await submit_queued_generation(
        client,
        session_factory,
        submission_key="queued-generation-for-isolation",
    )
    registered = await client.post(
        "/api/v1/auth/register",
        json={
            "email": "generation-cancel-other-owner@example.com",
            "password": "a-secure-generation-cancel-password",
            "display_name": "其他空间负责人",
        },
    )
    assert registered.status_code == 201
    other = registered.json()["data"]
    response = await client.post(
        f"/api/v1/tasks/{submitted['task']['id']}/cancel",
        headers={"authorization": f"Bearer {other['access_token']}"},
        json={
            "workspace_id": other["workspace"]["id"],
            "expected_revision": 1,
            "idempotency_key": "cross-workspace-cancel-command",
            "reason": "user_requested",
        },
    )
    assert response.status_code == 404

    async with session_factory() as session:
        task = await session.get(Task, UUID(submitted["task"]["id"]))
        reservation = await session.get(
            Reservation,
            UUID(submitted["reservation"]["id"]),
        )
        assert task is not None and task.status == "queued"
        assert reservation is not None and reservation.status == "active"
