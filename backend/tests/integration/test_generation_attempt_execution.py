from collections.abc import Awaitable, Callable
from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app import io_worker
from app.modules.governance.audit.models import AuditEvent
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production.models import (
    CostEntry,
    GenerationAttempt,
    Reservation,
    Task,
)
from tests.integration.test_generation_task_cancellation import (
    submit_queued_generation,
)


class RecordingMessage:
    def __init__(
        self,
        body: bytes,
        *,
        on_ack: Callable[[], Awaitable[None]] | None = None,
    ) -> None:
        self.body = body
        self.on_ack = on_ack
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        if self.on_ack is not None:
            await self.on_ack()
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


async def _submitted_generation_body(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    submission_key: str,
) -> tuple[dict[str, str], dict[str, Any], bytes]:
    headers, _, submitted = await submit_queued_generation(
        client,
        session_factory,
        submission_key=submission_key,
    )
    task_id = UUID(submitted["task"]["id"])
    async with session_factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(
                OutboxEvent.aggregate_id == task_id,
                OutboxEvent.event_type == "generation.requested",
            )
        )
        assert event is not None
        body = envelope_from_event(event).model_dump_json().encode("utf-8")
    return headers, submitted, body


@pytest.mark.asyncio
async def test_unconfigured_generation_dispatch_persists_attempt_before_ack_and_releases(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    _, submitted, body = await _submitted_generation_body(
        client,
        session_factory,
        submission_key="attempt-unconfigured-provider",
    )
    task_id = UUID(submitted["task"]["id"])

    async def assert_committed_before_ack() -> None:
        async with session_factory() as session:
            task = await session.get(Task, task_id)
            attempt = await session.scalar(
                select(GenerationAttempt).where(GenerationAttempt.task_id == task_id)
            )
            reservation = await session.get(
                Reservation,
                UUID(submitted["reservation"]["id"]),
            )
            assert reservation is not None
            release = await session.scalar(
                select(CostEntry).where(
                    CostEntry.reservation_id == reservation.id,
                    CostEntry.entry_type == "release",
                )
            )
            inbox = await session.scalar(
                select(InboxDelivery).where(InboxDelivery.task_id == task_id)
            )
            assert task is not None and task.status == "failed"
            assert attempt is not None and attempt.status == "failed"
            assert reservation.status == "released"
            assert release is not None and release.attempt_id == attempt.id
            assert inbox is not None and inbox.status == "completed"

    message = RecordingMessage(body, on_ack=assert_committed_before_ack)
    result = await io_worker.process_incoming_message(message, session_factory)

    assert result == "completed"
    assert message.ack_count == 1
    assert message.nack_requeues == []
    async with session_factory() as session:
        task = await session.get(Task, task_id)
        attempt = await session.scalar(
            select(GenerationAttempt).where(GenerationAttempt.task_id == task_id)
        )
        reservation = await session.get(
            Reservation,
            UUID(submitted["reservation"]["id"]),
        )
        assert attempt is not None
        assert reservation is not None
        release = await session.scalar(
            select(CostEntry).where(
                CostEntry.reservation_id == reservation.id,
                CostEntry.entry_type == "release",
            )
        )
        inbox = await session.scalar(
            select(InboxDelivery).where(InboxDelivery.task_id == task_id)
        )
        actions = list(
            await session.scalars(
                select(AuditEvent.action)
                .where(AuditEvent.target_id.in_((task_id, attempt.id)))
                .order_by(AuditEvent.occurred_at, AuditEvent.id)
            )
        )

        assert task is not None
        assert task.status == "failed"
        assert task.progress_stage == "blocked"
        assert task.error_code == "provider_dispatch_unavailable"
        assert task.error_retryable is False
        assert task.next_action == "wait_for_provider_activation"
        assert task.revision == 3
        assert attempt.sequence == 1
        assert attempt.status == "failed"
        assert attempt.provider_task_id is None
        assert attempt.submitted_at is None
        assert attempt.error_code == "provider_dispatch_unavailable"
        assert attempt.request_snapshot_hash == submitted["request"]["input_hash"]
        assert len(attempt.provider_request_key) == 64
        assert str(task_id) not in attempt.provider_request_key
        assert reservation.status == "released"
        assert reservation.revision == 2
        assert release is not None
        assert release.attempt_id == attempt.id
        assert release.amount == reservation.reserved_amount
        assert inbox is not None
        assert inbox.status == "completed"
        assert inbox.last_error == "provider_dispatch_unavailable"
        assert inbox.attempt_count == 1
        assert actions.count("task.started") == 1
        assert actions.count("task.failed") == 1
        assert actions.count("attempt.prepared") == 1
        assert actions.count("attempt.failed") == 1


@pytest.mark.asyncio
async def test_generation_dispatch_redelivery_is_fact_idempotent(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    _, submitted, body = await _submitted_generation_body(
        client,
        session_factory,
        submission_key="attempt-redelivery-idempotency",
    )
    first = RecordingMessage(body)
    second = RecordingMessage(body)

    assert await io_worker.process_incoming_message(first, session_factory) == "completed"
    assert await io_worker.process_incoming_message(second, session_factory) == "duplicate"
    assert first.ack_count == second.ack_count == 1
    assert first.nack_requeues == second.nack_requeues == []

    task_id = UUID(submitted["task"]["id"])
    reservation_id = UUID(submitted["reservation"]["id"])
    async with session_factory() as session:
        assert (
            await session.scalar(
                select(func.count())
                .select_from(GenerationAttempt)
                .where(GenerationAttempt.task_id == task_id)
            )
            == 1
        )
        assert (
            await session.scalar(
                select(func.count())
                .select_from(CostEntry)
                .where(
                    CostEntry.reservation_id == reservation_id,
                    CostEntry.entry_type == "release",
                )
            )
            == 1
        )
        inbox = await session.scalar(
            select(InboxDelivery).where(InboxDelivery.task_id == task_id)
        )
        assert inbox is not None and inbox.attempt_count == 2


@pytest.mark.asyncio
async def test_generation_dispatch_finalize_failure_reuses_prepared_attempt_on_redelivery(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    _, submitted, body = await _submitted_generation_body(
        client,
        session_factory,
        submission_key="attempt-finalize-recovery",
    )
    task_id = UUID(submitted["task"]["id"])
    original = io_worker.finalize_generation_dispatch_unavailable

    async def fail_finalize(*_: object, **__: object) -> None:
        raise RuntimeError("forced generation finalization rollback")

    monkeypatch.setattr(
        io_worker,
        "finalize_generation_dispatch_unavailable",
        fail_finalize,
    )
    first = RecordingMessage(body)
    assert await io_worker.process_incoming_message(first, session_factory) == "requeued"
    assert first.ack_count == 0
    assert first.nack_requeues == [True]

    async with session_factory() as session:
        task = await session.get(Task, task_id)
        prepared = await session.scalar(
            select(GenerationAttempt).where(GenerationAttempt.task_id == task_id)
        )
        reservation = await session.get(
            Reservation,
            UUID(submitted["reservation"]["id"]),
        )
        inbox = await session.scalar(
            select(InboxDelivery).where(InboxDelivery.task_id == task_id)
        )
        assert task is not None and task.status == "running"
        assert task.revision == 2
        assert prepared is not None and prepared.status == "prepared"
        prepared_id = prepared.id
        assert reservation is not None and reservation.status == "active"
        assert inbox is not None and inbox.status == "processing"

    monkeypatch.setattr(
        io_worker,
        "finalize_generation_dispatch_unavailable",
        original,
    )
    second = RecordingMessage(body)
    assert await io_worker.process_incoming_message(second, session_factory) == "completed"
    assert second.ack_count == 1
    assert second.nack_requeues == []

    async with session_factory() as session:
        attempts = list(
            await session.scalars(
                select(GenerationAttempt).where(GenerationAttempt.task_id == task_id)
            )
        )
        inbox = await session.scalar(
            select(InboxDelivery).where(InboxDelivery.task_id == task_id)
        )
        assert len(attempts) == 1
        assert attempts[0].id == prepared_id
        assert attempts[0].status == "failed"
        assert inbox is not None and inbox.attempt_count == 2


@pytest.mark.asyncio
async def test_cancelled_generation_event_acks_without_attempt_or_second_release(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, submitted, body = await _submitted_generation_body(
        client,
        session_factory,
        submission_key="attempt-cancelled-before-dispatch",
    )
    task = submitted["task"]
    cancelled = await client.post(
        f"/api/v1/tasks/{task['id']}/cancel",
        headers=headers,
        json={
            "workspace_id": task["workspace_id"],
            "expected_revision": task["revision"],
            "idempotency_key": "attempt-terminal-cancel",
            "reason": "user_requested",
        },
    )
    assert cancelled.status_code == 200

    message = RecordingMessage(body)
    assert await io_worker.process_incoming_message(message, session_factory) == "completed"
    assert message.ack_count == 1
    task_id = UUID(task["id"])
    reservation_id = UUID(submitted["reservation"]["id"])
    async with session_factory() as session:
        assert (
            await session.scalar(
                select(func.count())
                .select_from(GenerationAttempt)
                .where(GenerationAttempt.task_id == task_id)
            )
            == 0
        )
        assert (
            await session.scalar(
                select(func.count())
                .select_from(CostEntry)
                .where(
                    CostEntry.reservation_id == reservation_id,
                    CostEntry.entry_type == "release",
                )
            )
            == 1
        )
