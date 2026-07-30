from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.core.auth import decode_access_token
from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import ActorContext
from app.modules.messaging.models import OutboxEvent
from app.modules.production import ScriptExtractionTaskCommand, create_script_extraction_task
from app.modules.production.models import Task


async def _identity(
    client: httpx.AsyncClient,
    test_settings: Settings,
    *,
    email: str = "task-owner@example.com",
) -> tuple[dict[str, str], ActorContext]:
    response = await client.post(
        "/api/v1/auth/register",
        json={
            "email": email,
            "password": "a-secure-task-password",
            "display_name": "任务负责人",
        },
    )
    assert response.status_code == 201
    data = response.json()["data"]
    claims = decode_access_token(data["access_token"], test_settings)
    assert claims is not None
    return (
        {"authorization": f"Bearer {data['access_token']}"},
        ActorContext(
            user_id=claims.sub,
            workspace_id=UUID(data["workspace"]["id"]),
            membership_id=uuid7(),
            role="owner",
            workspace_status="active",
        ),
    )


def _command(workspace_id: UUID, *, input_hash: str = "a" * 64) -> ScriptExtractionTaskCommand:
    return ScriptExtractionTaskCommand(
        workspace_id=workspace_id,
        episode_id=uuid7(),
        request_id=uuid7(),
        input_version_id=uuid7(),
        input_hash=input_hash,
        idempotency_key="script-extraction-fixture-001",
    )


@pytest.mark.asyncio
async def test_script_extraction_task_and_outbox_are_atomic_and_idempotent(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    _, actor = await _identity(client, test_settings)
    command = _command(actor.workspace_id)

    async with session_factory() as session:
        async with session.begin():
            first = await create_script_extraction_task(
                session, actor, command, trace_id="trace-task-001"
            )
        async with session.begin():
            repeated = await create_script_extraction_task(
                session, actor, command, trace_id="trace-task-002"
            )

        assert repeated.id == first.id
        assert await session.scalar(select(func.count()).select_from(Task)) == 1
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 1
        event = await session.scalar(select(OutboxEvent))
        assert event is not None
        assert event.event_type == "script_extraction.requested"
        assert event.schema_version == 1
        assert event.aggregate_id == first.id
        assert event.workspace_id == actor.workspace_id
        assert event.payload == {"task_id": str(first.id)}
        assert command.input_hash not in str(event.payload)
        await session.rollback()

        with pytest.raises(ApiError) as conflict:
            async with session.begin():
                await create_script_extraction_task(
                    session,
                    actor,
                    command.model_copy(update={"input_hash": "b" * 64}),
                    trace_id="trace-task-003",
                )
        assert conflict.value.code == ErrorCode.RESOURCE_CONFLICT

    rolled_back = _command(actor.workspace_id, input_hash="c" * 64).model_copy(
        update={"idempotency_key": "rolled-back-command"}
    )
    async with session_factory() as session:
        with pytest.raises(RuntimeError, match="force rollback"):
            async with session.begin():
                await create_script_extraction_task(
                    session, actor, rolled_back, trace_id="trace-task-rollback"
                )
                raise RuntimeError("force rollback")
        assert (
            await session.scalar(
                select(func.count())
                .select_from(Task)
                .where(Task.idempotency_key == "rolled-back-command")
            )
            == 0
        )

    viewer = ActorContext(
        user_id=actor.user_id,
        workspace_id=actor.workspace_id,
        membership_id=uuid7(),
        role="viewer",
        workspace_status="active",
    )
    async with session_factory() as session:
        with pytest.raises(ApiError) as forbidden:
            async with session.begin():
                await create_script_extraction_task(
                    session,
                    viewer,
                    _command(actor.workspace_id).model_copy(
                        update={"idempotency_key": "viewer-command"}
                    ),
                    trace_id="trace-viewer",
                )
        assert forbidden.value.code == ErrorCode.FORBIDDEN
        assert await session.scalar(select(func.count()).select_from(Task)) == 1
        assert (
            await session.scalar(
                select(func.count())
                .select_from(OutboxEvent)
                .where(OutboxEvent.trace_id == "trace-task-rollback")
            )
            == 0
        )


@pytest.mark.asyncio
async def test_task_query_is_server_owned_paginated_and_workspace_isolated(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    owner_headers, owner = await _identity(client, test_settings)
    other_headers, _ = await _identity(
        client,
        test_settings,
        email="other-task-owner@example.com",
    )
    command = _command(owner.workspace_id)
    async with session_factory() as session:
        async with session.begin():
            task = await create_script_extraction_task(
                session, owner, command, trace_id="trace-query-001"
            )

    listed = await client.get(
        "/api/v1/tasks",
        headers=owner_headers,
        params={"workspace_id": str(owner.workspace_id), "limit": 1},
    )
    assert listed.status_code == 200
    page = listed.json()["data"]
    assert page["total"] == 1
    assert page["limit"] == 1
    assert page["offset"] == 0
    assert page["items"][0] == {
        "id": str(task.id),
        "workspace_id": str(owner.workspace_id),
        "task_type": "script_extraction",
        "request_type": "extraction_batch",
        "request_id": str(command.request_id),
        "scope": {
            "episode_id": str(command.episode_id),
            "render_snapshot_id": None,
            "usage_type": None,
            "usage_id": None,
            "input_version_id": str(command.input_version_id),
            "input_hash": command.input_hash,
        },
        "status": "queued",
        "progress_stage": "queued",
        "error": None,
        "next_action": "poll_task",
        "cancel_status": "none",
        "revision": 1,
    }

    fetched = await client.get(f"/api/v1/tasks/{task.id}", headers=owner_headers)
    assert fetched.status_code == 200
    hidden = await client.get(f"/api/v1/tasks/{task.id}", headers=other_headers)
    assert hidden.status_code == 404
    hidden_list = await client.get(
        "/api/v1/tasks",
        headers=other_headers,
        params={"workspace_id": str(owner.workspace_id)},
    )
    assert hidden_list.status_code == 404
    invalid_status = await client.get(
        "/api/v1/tasks",
        headers=owner_headers,
        params={"workspace_id": str(owner.workspace_id), "status": "invented"},
    )
    assert invalid_status.status_code == 422
    cannot_patch = await client.patch(
        f"/api/v1/tasks/{task.id}",
        headers=owner_headers,
        json={"status": "succeeded", "revision": 1},
    )
    assert cannot_patch.status_code == 405
