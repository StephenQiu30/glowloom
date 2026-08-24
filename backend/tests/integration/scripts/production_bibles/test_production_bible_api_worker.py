from datetime import UTC, datetime
from hashlib import sha256
from typing import Any, Literal
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.governance.audit.models import AuditEvent
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production.models import Task
from app.modules.scripts.production_bibles.checkpoints import (
    DatabaseProductionBibleCheckpointStore,
)
from app.modules.scripts.production_bibles.harness import (
    ProductionBibleCheckpoint,
    build_evidence_chunks,
)
from app.modules.scripts.production_bibles.models import (
    ProductionBible,
    ProductionBibleEntity,
    ProductionBibleEntityState,
    ProductionBibleWorldEntry,
)
from app.modules.scripts.production_bibles.ports import (
    PRODUCTION_BIBLE_HARNESS_VERSION,
    ProductionBibleInput,
    ProductionBibleProviderError,
)
from app.modules.scripts.production_bibles.schemas import (
    BibleAssetSpecCandidate,
    BibleEntityCandidate,
    BibleEntityStateCandidate,
    BibleEvidence,
    BibleEvidenceChunkResult,
    BibleEvidenceObservation,
    BibleWorldEntryCandidate,
    ProductionBibleProviderResult,
)
from app.runtime.workers import io as io_worker
from tests.integration.scripts.planning.test_episode_plans_api import (
    _project_and_document,  # pyright: ignore[reportPrivateUsage]
)

SOURCE_TEXT = (
    "第一集\n"
    "暴雨中的皇宫码头。\n"
    "艾琳穿着红色斗篷，握紧皇室钥匙。\n\n"
    "第二集\n"
    "战斗后，艾琳的红色斗篷已经撕裂。\n"
    "皇室钥匙可以开启封印之门。"
)


class _RecordingMessage:
    def __init__(self, body: bytes) -> None:
        self.body = body
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


def _evidence(
    source: str,
    anchor: str,
    *,
    episode_number: int,
) -> BibleEvidence:
    source_start = source.index(anchor)
    return BibleEvidence(
        source_start=source_start,
        source_end=source_start + len(anchor),
        text_hash=sha256(anchor.encode("utf-8")).hexdigest(),
        exact_anchor=anchor,
        episode_number=episode_number,
    )


def _provider_result(source: str) -> ProductionBibleProviderResult:
    identity_evidence = _evidence(
        source,
        "艾琳穿着红色斗篷，握紧皇室钥匙。",
        episode_number=1,
    )
    state_evidence = _evidence(
        source,
        "艾琳的红色斗篷已经撕裂",
        episode_number=2,
    )
    key_evidence = _evidence(source, "皇室钥匙", episode_number=1)
    world_evidence = _evidence(
        source,
        "皇室钥匙可以开启封印之门。",
        episode_number=2,
    )
    return ProductionBibleProviderResult(
        entities=[
            BibleEntityCandidate(
                entity_key="character.aileen",
                kind="character",
                canonical_name="艾琳",
                normalized_name="艾琳",
                aliases=["女皇"],
                stable_spec=BibleAssetSpecCandidate(identity="失踪的女皇"),
                episode_numbers=[1, 2],
                evidence=[identity_evidence],
                states=[
                    BibleEntityStateCandidate(
                        state_key="battle_torn_cloak",
                        label="战损红斗篷",
                        state_spec=BibleAssetSpecCandidate(appearance="torn cloak"),
                        episode_numbers=[2],
                        evidence=[state_evidence],
                    )
                ],
            ),
            BibleEntityCandidate(
                entity_key="prop.royal_key",
                kind="prop",
                canonical_name="皇室钥匙",
                normalized_name="皇室钥匙",
                stable_spec=BibleAssetSpecCandidate(usage_context="开启封印之门"),
                episode_numbers=[1, 2],
                evidence=[key_evidence],
            ),
        ],
        world_entries=[
            BibleWorldEntryCandidate(
                entry_key="rule.sealed_gate",
                category="world_rule",
                title="封印之门开启规则",
                rules=["只有皇室钥匙可以开启封印之门。"],
                entity_keys=["prop.royal_key"],
                episode_numbers=[2],
                evidence=[world_evidence],
            )
        ],
    )


class _RecordingProductionBibleBuilder:
    def __init__(self) -> None:
        self.inputs: list[ProductionBibleInput] = []

    async def build(
        self,
        bible_input: ProductionBibleInput,
    ) -> ProductionBibleProviderResult:
        self.inputs.append(bible_input)
        return _provider_result(bible_input.normalized_text)


class _CheckpointingFailureBuilder:
    def __init__(
        self,
        session_factory: async_sessionmaker[AsyncSession],
        *,
        outcome: Literal["failed", "unknown"],
    ) -> None:
        self._store = DatabaseProductionBibleCheckpointStore(session_factory)
        self._outcome: Literal["failed", "unknown"] = outcome
        self.inputs: list[ProductionBibleInput] = []

    async def build(
        self,
        bible_input: ProductionBibleInput,
    ) -> ProductionBibleProviderResult:
        self.inputs.append(bible_input)
        identity_evidence = _evidence(
            bible_input.normalized_text,
            "艾琳穿着红色斗篷，握紧皇室钥匙。",
            episode_number=1,
        )
        chunk_results: list[BibleEvidenceChunkResult] = []
        for chunk in build_evidence_chunks(bible_input.normalized_text):
            observations: list[BibleEvidenceObservation] = []
            if (
                chunk.source_start <= identity_evidence.source_start
                and identity_evidence.source_end <= chunk.source_end
            ):
                observations.append(
                    BibleEvidenceObservation(
                        observation_key="observation.character.aileen",
                        kind="entity",
                        subject_key="character.aileen",
                        claim="艾琳是同一个稳定角色身份。",
                        evidence=[identity_evidence],
                    )
                )
            chunk_results.append(
                BibleEvidenceChunkResult(
                    chunk_key=chunk.key,
                    source_start=chunk.source_start,
                    source_end=chunk.source_end,
                    observations=observations,
                )
            )
        checkpoint = ProductionBibleCheckpoint(
            bible_id=bible_input.bible_id,
            task_id=bible_input.task_id,
            run_token=bible_input.run_token,
            input_hash=bible_input.input_hash,
            harness_version=PRODUCTION_BIBLE_HARNESS_VERSION,
            stage="evidence",
            completed_chunk_keys=tuple(item.chunk_key for item in chunk_results),
            evidence_chunks=tuple(chunk_results),
            updated_at=datetime.now(UTC),
        )
        await self._store.save(checkpoint)
        raise ProductionBibleProviderError(
            outcome=self._outcome,
            code=f"provider_{self._outcome}",
            summary=f"Provider ended as {self._outcome}",
            retryable=True,
            next_action="resume_production_bible",
        )


class _InspectingResumeBuilder:
    def __init__(
        self,
        session_factory: async_sessionmaker[AsyncSession],
    ) -> None:
        self._store = DatabaseProductionBibleCheckpointStore(session_factory)
        self.inputs: list[ProductionBibleInput] = []
        self.loaded: list[ProductionBibleCheckpoint] = []

    async def build(
        self,
        bible_input: ProductionBibleInput,
    ) -> ProductionBibleProviderResult:
        self.inputs.append(bible_input)
        checkpoint = await self._store.load_latest(
            bible_input.bible_id,
            bible_input.input_hash,
        )
        if checkpoint is None:
            raise RuntimeError("Formal resume did not expose the persisted checkpoint")
        self.loaded.append(checkpoint)
        return _provider_result(bible_input.normalized_text)


@pytest.mark.asyncio
async def test_api_outbox_worker_persists_reviewable_production_bible(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, project, document = await _project_and_document(
        client,
        email="production-bible-worker@example.com",
        text=SOURCE_TEXT,
    )
    revision = document["revision"]
    created_response = await client.post(
        f"/api/v1/document-revisions/{revision['id']}/production-bibles",
        headers=headers,
        json={"idempotency_key": "production-bible-worker-001"},
    )

    assert created_response.status_code == 202
    created: dict[str, Any] = created_response.json()["data"]
    assert created["project_id"] == project["id"]
    assert created["document_revision_id"] == revision["id"]
    assert created["status"] == "queued"
    assert created["task_id"] is not None
    assert created["entities"] == []
    assert created["world_entries"] == []

    task_id = UUID(created["task_id"])
    async with session_factory() as session:
        event = await session.scalar(select(OutboxEvent).where(OutboxEvent.aggregate_id == task_id))
        assert event is not None
        assert event.event_type == "production_bible.requested"
        assert event.payload == {"task_id": str(task_id)}
        message_body = envelope_from_event(event).model_dump_json().encode("utf-8")

    builder = _RecordingProductionBibleBuilder()
    message = _RecordingMessage(message_body)
    outcome = await io_worker.process_incoming_message(
        message,
        session_factory,
        production_bible_builder=builder,
    )

    assert outcome == "completed"
    assert message.ack_count == 1
    assert message.nack_requeues == []
    assert len(builder.inputs) == 1
    assert builder.inputs[0].normalized_text == SOURCE_TEXT
    assert builder.inputs[0].input_hash == revision["normalized_hash"]

    fetched_response = await client.get(
        f"/api/v1/production-bibles/{created['id']}",
        headers=headers,
    )
    assert fetched_response.status_code == 200
    fetched: dict[str, Any] = fetched_response.json()["data"]
    assert fetched["status"] == "needs_review"
    assert fetched["result_hash"] is not None
    assert [entity["entity_key"] for entity in fetched["entities"]] == [
        "character.aileen",
        "prop.royal_key",
    ]
    assert fetched["entities"][0]["states"][0]["state_key"] == ("battle_torn_cloak")
    assert fetched["world_entries"][0]["entry_key"] == "rule.sealed_gate"

    async with session_factory() as session:
        bible = await session.get(ProductionBible, UUID(created["id"]))
        task = await session.get(Task, task_id)
        entities = list(
            await session.scalars(
                select(ProductionBibleEntity).where(
                    ProductionBibleEntity.bible_id == UUID(created["id"])
                )
            )
        )
        states = list(
            await session.scalars(
                select(ProductionBibleEntityState).where(
                    ProductionBibleEntityState.bible_id == UUID(created["id"])
                )
            )
        )
        world_entries = list(
            await session.scalars(
                select(ProductionBibleWorldEntry).where(
                    ProductionBibleWorldEntry.bible_id == UUID(created["id"])
                )
            )
        )
        delivery = await session.scalar(
            select(InboxDelivery).where(InboxDelivery.task_id == task_id)
        )

        assert bible is not None
        assert bible.status == "needs_review"
        assert bible.run_token is None
        assert bible.lease_expires_at is None
        assert task is not None
        assert task.status == "succeeded"
        assert task.progress_stage == "completed"
        assert len(entities) == 2
        assert len(states) == 1
        assert len(world_entries) == 1
        assert states[0].state_key == "battle_torn_cloak"
        assert world_entries[0].entity_keys == ["prop.royal_key"]
        assert delivery is not None
        assert delivery.status == "completed"
        assert delivery.attempt_count == 1


@pytest.mark.parametrize("terminal_status", ["failed", "unknown"])
@pytest.mark.asyncio
async def test_resume_reuses_checkpoint_with_a_new_task_and_preserves_history(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    terminal_status: Literal["failed", "unknown"],
) -> None:
    headers, _, document = await _project_and_document(
        client,
        email=f"production-bible-resume-{terminal_status}@example.com",
        text=SOURCE_TEXT,
    )
    revision = document["revision"]
    created_response = await client.post(
        f"/api/v1/document-revisions/{revision['id']}/production-bibles",
        headers=headers,
        json={"idempotency_key": f"resume-source-{terminal_status}"},
    )
    assert created_response.status_code == 202
    created: dict[str, Any] = created_response.json()["data"]
    bible_id = UUID(created["id"])
    previous_task_id = UUID(created["task_id"])

    async with session_factory() as session:
        previous_event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.aggregate_id == previous_task_id)
        )
        assert previous_event is not None
        previous_body = envelope_from_event(previous_event).model_dump_json().encode()

    failed_builder = _CheckpointingFailureBuilder(
        session_factory,
        outcome=terminal_status,
    )
    previous_message = _RecordingMessage(previous_body)
    previous_outcome = await io_worker.process_incoming_message(
        previous_message,
        session_factory,
        production_bible_builder=failed_builder,
    )
    assert previous_outcome == "completed"
    assert previous_message.ack_count == 1
    assert len(failed_builder.inputs) == 1

    terminal_response = await client.get(
        f"/api/v1/production-bibles/{bible_id}",
        headers=headers,
    )
    assert terminal_response.status_code == 200
    terminal: dict[str, Any] = terminal_response.json()["data"]
    assert terminal["status"] == terminal_status
    assert terminal["checkpoint_stage"] == "evidence"
    assert terminal["checkpoint_revision"] == 1
    terminal_revision = terminal["revision"]

    stale_response = await client.post(
        f"/api/v1/production-bibles/{bible_id}/resume",
        headers=headers,
        json={
            "expected_revision": terminal_revision - 1,
            "idempotency_key": "formal-resume-001",
        },
    )
    assert stale_response.status_code == 409
    assert stale_response.json()["error"]["code"] == "version_conflict"

    resumed_response = await client.post(
        f"/api/v1/production-bibles/{bible_id}/resume",
        headers=headers,
        json={
            "expected_revision": terminal_revision,
            "idempotency_key": "formal-resume-001",
        },
    )
    assert resumed_response.status_code == 202
    resumed: dict[str, Any] = resumed_response.json()["data"]
    resumed_task_id = UUID(resumed["task_id"])
    assert resumed["id"] == str(bible_id)
    assert resumed["status"] == "queued"
    assert resumed_task_id != previous_task_id
    assert resumed["revision"] == terminal_revision + 1
    assert resumed["checkpoint_stage"] == terminal["checkpoint_stage"]
    assert resumed["checkpoint_revision"] == terminal["checkpoint_revision"]

    replay_response = await client.post(
        f"/api/v1/production-bibles/{bible_id}/resume",
        headers=headers,
        json={
            "expected_revision": terminal_revision,
            "idempotency_key": "formal-resume-001",
        },
    )
    assert replay_response.status_code == 202
    replayed: dict[str, Any] = replay_response.json()["data"]
    assert replayed["task_id"] == str(resumed_task_id)
    assert replayed["revision"] == resumed["revision"]

    reused_with_different_input = await client.post(
        f"/api/v1/production-bibles/{bible_id}/resume",
        headers=headers,
        json={
            "expected_revision": resumed["revision"],
            "idempotency_key": "formal-resume-001",
        },
    )
    assert reused_with_different_input.status_code == 409
    assert reused_with_different_input.json()["error"]["code"] == "resource_conflict"

    async with session_factory() as session:
        resumed_event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.aggregate_id == resumed_task_id)
        )
        assert resumed_event is not None
        resumed_body = envelope_from_event(resumed_event).model_dump_json().encode()
        production_task_count = await session.scalar(
            select(func.count())
            .select_from(Task)
            .where(
                Task.task_type == "production_bible",
                Task.request_id == bible_id,
            )
        )
        production_outbox_count = await session.scalar(
            select(func.count())
            .select_from(OutboxEvent)
            .where(OutboxEvent.event_type == "production_bible.requested")
        )
        assert production_task_count == 2
        assert production_outbox_count == 2

    resumed_builder = _InspectingResumeBuilder(session_factory)
    resumed_message = _RecordingMessage(resumed_body)
    resumed_outcome = await io_worker.process_incoming_message(
        resumed_message,
        session_factory,
        production_bible_builder=resumed_builder,
    )
    assert resumed_outcome == "completed"
    assert resumed_message.ack_count == 1
    assert len(resumed_builder.loaded) == 1
    loaded_checkpoint = resumed_builder.loaded[0]
    assert loaded_checkpoint.task_id == resumed_task_id
    assert loaded_checkpoint.run_token == resumed_builder.inputs[0].run_token
    assert loaded_checkpoint.evidence_chunks[0].observations[0].evidence

    async with session_factory() as session:
        bible = await session.get(ProductionBible, bible_id)
        previous_task = await session.get(Task, previous_task_id)
        resumed_task = await session.get(Task, resumed_task_id)
        previous_delivery = await session.scalar(
            select(InboxDelivery).where(InboxDelivery.task_id == previous_task_id)
        )
        resumed_delivery = await session.scalar(
            select(InboxDelivery).where(InboxDelivery.task_id == resumed_task_id)
        )
        resume_audit = await session.scalar(
            select(AuditEvent).where(
                AuditEvent.action == "script.production_bible_resumed",
                AuditEvent.target_id == bible_id,
            )
        )
        assert bible is not None
        assert bible.status == "needs_review"
        assert bible.checkpoint is not None
        assert bible.checkpoint["task_id"] == str(previous_task_id)
        assert bible.checkpoint_revision == 1
        assert previous_task is not None
        assert previous_task.status == terminal_status
        assert previous_task.error_code == f"provider_{terminal_status}"
        assert previous_task.error_retryable is True
        assert previous_task.next_action == "resume_production_bible"
        assert resumed_task is not None
        assert resumed_task.status == "succeeded"
        assert previous_delivery is not None
        assert previous_delivery.status == "completed"
        assert previous_delivery.attempt_count == 1
        assert resumed_delivery is not None
        assert resumed_delivery.status == "completed"
        assert resumed_delivery.attempt_count == 1
        assert resume_audit is not None
        assert resume_audit.event_metadata == {
            "previous_status": terminal_status,
            "previous_task_id": str(previous_task_id),
            "task_id": str(resumed_task_id),
            "revision": terminal_revision + 1,
            "checkpoint_revision": 1,
        }

    invalid_state_response = await client.post(
        f"/api/v1/production-bibles/{bible_id}/resume",
        headers=headers,
        json={
            "expected_revision": resumed["revision"] + 2,
            "idempotency_key": "formal-resume-002",
        },
    )
    assert invalid_state_response.status_code == 409
    assert invalid_state_response.json()["error"]["code"] == "state_conflict"


@pytest.mark.asyncio
async def test_resume_without_checkpoint_fails_closed_without_creating_a_task(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, document = await _project_and_document(
        client,
        email="production-bible-resume-no-checkpoint@example.com",
        text=SOURCE_TEXT,
    )
    revision = document["revision"]
    created_response = await client.post(
        f"/api/v1/document-revisions/{revision['id']}/production-bibles",
        headers=headers,
        json={"idempotency_key": "resume-no-checkpoint-source"},
    )
    assert created_response.status_code == 202
    created: dict[str, Any] = created_response.json()["data"]
    task_id = UUID(created["task_id"])

    async with session_factory() as session:
        event = await session.scalar(select(OutboxEvent).where(OutboxEvent.aggregate_id == task_id))
        assert event is not None
        body = envelope_from_event(event).model_dump_json().encode()
    message = _RecordingMessage(body)
    outcome = await io_worker.process_incoming_message(
        message,
        session_factory,
        production_bible_builder=None,
    )
    assert outcome == "completed"

    failed_response = await client.get(
        f"/api/v1/production-bibles/{created['id']}",
        headers=headers,
    )
    failed: dict[str, Any] = failed_response.json()["data"]
    assert failed["status"] == "failed"
    assert failed["checkpoint_revision"] == 0

    resume_response = await client.post(
        f"/api/v1/production-bibles/{created['id']}/resume",
        headers=headers,
        json={
            "expected_revision": failed["revision"],
            "idempotency_key": "resume-no-checkpoint-001",
        },
    )
    assert resume_response.status_code == 409
    assert resume_response.json()["error"]["code"] == "state_conflict"

    async with session_factory() as session:
        task_count = await session.scalar(
            select(func.count())
            .select_from(Task)
            .where(
                Task.task_type == "production_bible",
                Task.request_id == UUID(created["id"]),
            )
        )
        assert task_count == 1
