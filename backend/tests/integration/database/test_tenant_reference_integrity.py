from datetime import UTC, datetime

import pytest
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.identity.models import UserAccount, Workspace
from app.modules.messaging.models import OutboxEvent
from app.modules.production.models import GenerationAttempt, Task
from app.modules.projects.models import Episode, Project
from app.modules.scheduling.models import Schedule, ScheduleFire
from app.modules.storyboards.models import ShotTransform


@pytest.mark.asyncio
async def test_database_rejects_schedule_fire_with_cross_workspace_outbox(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    owner_id = uuid7()
    workspace_id = uuid7()
    other_workspace_id = uuid7()
    schedule_id = uuid7()
    task_id = uuid7()
    outbox_event_id = uuid7()
    now = datetime.now(UTC)

    async with session_factory() as session:
        async with session.begin():
            session.add_all(
                (
                    UserAccount(
                        id=owner_id,
                        email_normalized=f"tenant-integrity-{owner_id}@example.com",
                        password_hash="synthetic-not-used",
                        display_name="Tenant Integrity Fixture",
                    ),
                    Workspace(id=workspace_id, name="Scheduling Workspace"),
                    Workspace(id=other_workspace_id, name="Foreign Outbox Workspace"),
                )
            )
            await session.flush()
            session.add_all(
                (
                    Schedule(
                        id=schedule_id,
                        workspace_id=workspace_id,
                        schedule_key="tenant-integrity-schedule",
                        handler_name="media.expire_upload",
                        scope={"workspace_id": str(workspace_id)},
                        payload={},
                        kind="one_off",
                        rule={"at": now.isoformat()},
                        next_fire_at=now,
                        created_by=owner_id,
                    ),
                    Task(
                        id=task_id,
                        workspace_id=workspace_id,
                        task_type="upload_expiration",
                        request_type="schedule_fire",
                        request_id=schedule_id,
                        idempotency_key="tenant-integrity-task",
                        requested_by=owner_id,
                    ),
                    OutboxEvent(
                        id=outbox_event_id,
                        workspace_id=other_workspace_id,
                        event_type="media.upload.expiration.requested",
                        schema_version=1,
                        aggregate_type="task",
                        aggregate_id=task_id,
                        topic="lanverse.media.v1",
                        payload={"task_id": str(task_id)},
                        trace_id="trace-tenant-integrity-outbox",
                        traceparent=("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"),
                    ),
                )
            )
            await session.flush()

        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    ScheduleFire(
                        workspace_id=workspace_id,
                        schedule_id=schedule_id,
                        fire_key="scheduled:tenant-integrity",
                        scheduled_for=now,
                        trigger_kind="scheduled",
                        task_id=task_id,
                        outbox_event_id=outbox_event_id,
                        trace_id="trace-tenant-integrity-fire",
                    )
                )
                await session.flush()


@pytest.mark.asyncio
async def test_database_rejects_shot_transform_with_cross_workspace_episode(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    owner_id = uuid7()
    workspace_id = uuid7()
    other_workspace_id = uuid7()
    project_id = uuid7()
    episode_id = uuid7()

    async with session_factory() as session:
        async with session.begin():
            session.add_all(
                (
                    UserAccount(
                        id=owner_id,
                        email_normalized=f"transform-integrity-{owner_id}@example.com",
                        password_hash="synthetic-not-used",
                        display_name="Transform Integrity Fixture",
                    ),
                    Workspace(id=workspace_id, name="Transform Workspace"),
                    Workspace(id=other_workspace_id, name="Foreign Episode Workspace"),
                )
            )
            await session.flush()
            session.add(
                Project(
                    id=project_id,
                    workspace_id=other_workspace_id,
                    name="Foreign Project",
                    aspect_ratio="9:16",
                    language="zh-CN",
                    target_duration_ms=90_000,
                )
            )
            await session.flush()
            session.add(
                Episode(
                    id=episode_id,
                    workspace_id=other_workspace_id,
                    project_id=project_id,
                    name="Foreign Episode",
                    position=1,
                    target_duration_ms=90_000,
                )
            )
            await session.flush()

        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    ShotTransform(
                        workspace_id=workspace_id,
                        episode_id=episode_id,
                        operation="copy",
                        source_shot_ids=[],
                        source_spec_version_ids=[],
                        result_shot_ids=[],
                        impact_hash="a" * 64,
                        input_hash="b" * 64,
                        idempotency_key="cross-workspace-transform",
                        actor_id=owner_id,
                    )
                )
                await session.flush()


@pytest.mark.asyncio
async def test_database_rejects_generation_attempt_with_cross_workspace_task(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    owner_id = uuid7()
    workspace_id = uuid7()
    other_workspace_id = uuid7()
    task_id = uuid7()
    now = datetime.now(UTC)

    async with session_factory() as session:
        async with session.begin():
            session.add_all(
                (
                    UserAccount(
                        id=owner_id,
                        email_normalized=f"attempt-integrity-{owner_id}@example.com",
                        password_hash="synthetic-not-used",
                        display_name="Attempt Integrity Fixture",
                    ),
                    Workspace(id=workspace_id, name="Attempt Workspace"),
                    Workspace(id=other_workspace_id, name="Foreign Attempt Workspace"),
                )
            )
            await session.flush()
            session.add(
                Task(
                    id=task_id,
                    workspace_id=workspace_id,
                    task_type="image_generation",
                    request_type="generation_request",
                    request_id=uuid7(),
                    input_hash="a" * 64,
                    idempotency_key="attempt-integrity-task",
                    requested_by=owner_id,
                )
            )
            await session.flush()

        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    GenerationAttempt(
                        workspace_id=other_workspace_id,
                        task_id=task_id,
                        sequence=1,
                        provider_request_key="b" * 64,
                        status="prepared",
                        request_snapshot_hash="a" * 64,
                        prepared_at=now,
                        updated_at=now,
                    )
                )
                await session.flush()


@pytest.mark.parametrize(
    ("second_sequence", "second_provider_key"),
    ((1, "c" * 64), (2, "b" * 64)),
)
@pytest.mark.asyncio
async def test_database_rejects_duplicate_generation_attempt_identity(
    session_factory: async_sessionmaker[AsyncSession],
    second_sequence: int,
    second_provider_key: str,
) -> None:
    owner_id = uuid7()
    workspace_id = uuid7()
    task_id = uuid7()
    now = datetime.now(UTC)

    async with session_factory() as session:
        async with session.begin():
            session.add_all(
                (
                    UserAccount(
                        id=owner_id,
                        email_normalized=f"attempt-unique-{owner_id}@example.com",
                        password_hash="synthetic-not-used",
                        display_name="Attempt Unique Fixture",
                    ),
                    Workspace(id=workspace_id, name="Attempt Unique Workspace"),
                )
            )
            await session.flush()
            session.add(
                Task(
                    id=task_id,
                    workspace_id=workspace_id,
                    task_type="image_generation",
                    request_type="generation_request",
                    request_id=uuid7(),
                    input_hash="a" * 64,
                    idempotency_key="attempt-unique-task",
                    requested_by=owner_id,
                )
            )
            await session.flush()
            session.add(
                GenerationAttempt(
                    workspace_id=workspace_id,
                    task_id=task_id,
                    sequence=1,
                    provider_request_key="b" * 64,
                    status="prepared",
                    request_snapshot_hash="a" * 64,
                    prepared_at=now,
                    updated_at=now,
                )
            )
            await session.flush()

        with pytest.raises(IntegrityError):
            async with session.begin():
                session.add(
                    GenerationAttempt(
                        workspace_id=workspace_id,
                        task_id=task_id,
                        sequence=second_sequence,
                        provider_request_key=second_provider_key,
                        status="prepared",
                        request_snapshot_hash="a" * 64,
                        prepared_at=now,
                        updated_at=now,
                    )
                )
                await session.flush()
