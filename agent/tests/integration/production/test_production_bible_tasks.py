from datetime import UTC, datetime
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.core.auth import decode_access_token
from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit.models import AuditEvent
from app.modules.identity import ActorContext
from app.modules.messaging import IO_TOPIC
from app.modules.messaging.models import OutboxEvent
from app.modules.production import (
    ProductionBibleTaskCommand,
    TaskResponse,
    complete_production_bible_task,
    create_production_bible_task,
    fail_production_bible_task,
    mark_production_bible_task_unknown,
    start_production_bible_task,
)
from app.modules.production.models import Task
from tests.support.identity_builders import register_identity_response


async def _actor(
    client: httpx.AsyncClient,
    test_settings: Settings,
) -> ActorContext:
    response = await register_identity_response(
        client,
        email="production-bible-task@example.com",
        password="a-secure-production-bible-password",
        display_name="Bible Task Owner",
    )
    assert response.status_code == 201
    claims = decode_access_token(response.json()["data"]["access_token"], test_settings)
    assert claims is not None
    return ActorContext(
        user_id=claims.sub,
        workspace_id=UUID(response.json()["data"]["workspace"]["id"]),
        membership_id=uuid7(),
        role="owner",
        workspace_status="active",
    )


def _command(
    workspace_id: UUID,
    *,
    idempotency_key: str,
) -> ProductionBibleTaskCommand:
    return ProductionBibleTaskCommand(
        workspace_id=workspace_id,
        bible_id=uuid7(),
        document_revision_id=uuid7(),
        input_hash="a" * 64,
        idempotency_key=idempotency_key,
    )


@pytest.mark.asyncio
async def test_production_bible_task_is_document_scoped_atomic_and_idempotent(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    actor = await _actor(client, test_settings)
    command = _command(actor.workspace_id, idempotency_key="production-bible-001")

    async with session_factory() as session:
        async with session.begin():
            first = await create_production_bible_task(
                session,
                actor,
                command,
                trace_id="trace-production-bible-create",
            )
        async with session.begin():
            repeated = await create_production_bible_task(
                session,
                actor,
                command,
                trace_id="trace-production-bible-repeat",
            )

        assert repeated.id == first.id
        assert first.task_type == "production_bible"
        assert first.request_type == "production_bible"
        assert first.request_id == command.bible_id
        assert first.scope.episode_id is None
        assert first.scope.usage_type == "document_revision"
        assert first.scope.usage_id == command.document_revision_id
        assert first.scope.input_version_id == command.document_revision_id
        assert first.scope.input_hash == command.input_hash
        assert await session.scalar(select(func.count()).select_from(Task)) == 1
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 1

        event = await session.scalar(select(OutboxEvent))
        assert event is not None
        assert event.event_type == "production_bible.requested"
        assert event.schema_version == 1
        assert event.aggregate_type == "task"
        assert event.aggregate_id == first.id
        assert event.topic == IO_TOPIC
        assert event.payload == {"task_id": str(first.id)}
        assert command.input_hash not in str(event.payload)

        audit = await session.scalar(
            select(AuditEvent).where(
                AuditEvent.target_type == "task",
                AuditEvent.target_id == first.id,
            )
        )
        assert audit is not None
        assert audit.action == "task.created"
        assert audit.trace_id == "trace-production-bible-create"
        assert audit.event_metadata == {
            "revision": 1,
            "task_type": "production_bible",
            "request_type": "production_bible",
            "request_id": str(command.bible_id),
        }
        await session.rollback()

        with pytest.raises(ApiError) as conflict:
            async with session.begin():
                await create_production_bible_task(
                    session,
                    actor,
                    command.model_copy(update={"bible_id": uuid7()}),
                    trace_id="trace-production-bible-conflict",
                )
        assert conflict.value.code == ErrorCode.RESOURCE_CONFLICT


@pytest.mark.asyncio
async def test_production_bible_task_lifecycle_records_audited_terminal_states(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    actor = await _actor(client, test_settings)
    commands: dict[str, ProductionBibleTaskCommand] = {
        outcome: _command(
            actor.workspace_id,
            idempotency_key=f"production-bible-{outcome}",
        )
        for outcome in ("succeeded", "failed", "unknown")
    }

    async with session_factory() as session:
        created: dict[str, TaskResponse] = {}
        for outcome, command in commands.items():
            async with session.begin():
                created[outcome] = await create_production_bible_task(
                    session,
                    actor,
                    command,
                    trace_id=f"trace-production-bible-{outcome}-created",
                )

        for outcome, task in created.items():
            async with session.begin():
                assert await start_production_bible_task(
                    session,
                    task.id,
                    now=datetime.now(UTC),
                    trace_id=f"trace-production-bible-{outcome}-started",
                )
            async with session.begin():
                assert not await start_production_bible_task(
                    session,
                    task.id,
                    now=datetime.now(UTC),
                    trace_id=f"trace-production-bible-{outcome}-start-repeat",
                )

        async with session.begin():
            assert await complete_production_bible_task(
                session,
                created["succeeded"].id,
                now=datetime.now(UTC),
                trace_id="trace-production-bible-succeeded",
            )
        async with session.begin():
            assert not await complete_production_bible_task(
                session,
                created["succeeded"].id,
                now=datetime.now(UTC),
                trace_id="trace-production-bible-succeeded-repeat",
            )

        async with session.begin():
            assert await fail_production_bible_task(
                session,
                created["failed"].id,
                error_code="bible_output_invalid",
                error_summary="Candidate output did not pass deterministic validation",
                next_action="review_production_bible_failure",
                retryable=False,
                now=datetime.now(UTC),
                trace_id="trace-production-bible-failed",
            )
        async with session.begin():
            assert not await fail_production_bible_task(
                session,
                created["failed"].id,
                error_code="ignored",
                error_summary="ignored",
                next_action="ignored",
                now=datetime.now(UTC),
                trace_id="trace-production-bible-failed-repeat",
            )

        async with session.begin():
            assert await mark_production_bible_task_unknown(
                session,
                created["unknown"].id,
                now=datetime.now(UTC),
                trace_id="trace-production-bible-unknown",
                error_code="provider_response_unknown",
                error_summary="Provider may have completed the request",
                retryable=True,
                next_action="resume_production_bible",
            )
        async with session.begin():
            assert not await mark_production_bible_task_unknown(
                session,
                created["unknown"].id,
                now=datetime.now(UTC),
                trace_id="trace-production-bible-unknown-repeat",
            )

        stored: dict[str, Task] = {}
        for outcome, task in created.items():
            stored_task = await session.get(Task, task.id, populate_existing=True)
            assert stored_task is not None
            stored[outcome] = stored_task

        assert stored["succeeded"].status == "succeeded"
        assert stored["succeeded"].progress_stage == "completed"
        assert stored["succeeded"].next_action == "review_production_bible"
        assert stored["succeeded"].revision == 3

        assert stored["failed"].status == "failed"
        assert stored["failed"].progress_stage == "blocked"
        assert stored["failed"].error_code == "bible_output_invalid"
        assert stored["failed"].error_retryable is False
        assert stored["failed"].next_action == "review_production_bible_failure"
        assert stored["failed"].revision == 3

        assert stored["unknown"].status == "unknown"
        assert stored["unknown"].progress_stage == "reconciliation_required"
        assert stored["unknown"].error_code == "provider_response_unknown"
        assert stored["unknown"].error_summary == "Provider may have completed the request"
        assert stored["unknown"].error_retryable is True
        assert stored["unknown"].next_action == "resume_production_bible"
        assert stored["unknown"].revision == 3

        expected_actions = {
            "succeeded": ["task.created", "task.started", "task.succeeded"],
            "failed": ["task.created", "task.started", "task.failed"],
            "unknown": ["task.created", "task.started", "task.unknown"],
        }
        for outcome, task in created.items():
            events = list(
                await session.scalars(
                    select(AuditEvent)
                    .where(
                        AuditEvent.target_type == "task",
                        AuditEvent.target_id == task.id,
                    )
                    .order_by(AuditEvent.occurred_at, AuditEvent.id)
                )
            )
            assert [event.action for event in events] == expected_actions[outcome]
            assert events[1].event_metadata["previous_status"] == "queued"
            assert events[1].event_metadata["status"] == "running"
            assert events[2].event_metadata["previous_status"] == "running"
            assert events[2].event_metadata["status"] == outcome

        unknown_events = list(
            await session.scalars(
                select(AuditEvent)
                .where(
                    AuditEvent.target_type == "task",
                    AuditEvent.target_id == created["unknown"].id,
                )
                .order_by(AuditEvent.occurred_at, AuditEvent.id)
            )
        )
        assert unknown_events[-1].event_metadata["error_code"] == "provider_response_unknown"
        assert "Provider may have completed the request" not in str(
            unknown_events[-1].event_metadata
        )
