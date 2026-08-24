from collections.abc import AsyncIterator
from datetime import UTC, datetime, timedelta
from typing import Any
from uuid import UUID

import httpx
import pytest
import pytest_asyncio
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.storyboards.agents import (
    STORYBOARD_AGENT_HARNESS_VERSION,
    DatabaseStoryboardCheckpointStore,
    SceneContext,
    SceneContextUnit,
    SceneDraft,
    StoryboardCheckpoint,
    StoryboardCheckpointMismatchError,
    assemble_storyboard,
)
from app.modules.storyboards.drafts.models import StoryboardDraftBatch
from app.modules.storyboards.drafts.provider_schema import StoryboardProviderResult
from tests.integration.storyboards.test_draft_batches import (
    create_batch_fixture,
    published_episode,
)


@pytest_asyncio.fixture
async def draft_batch(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> AsyncIterator[StoryboardDraftBatch]:
    headers, _project, episode, script = await published_episode(
        client,
        session_factory,
        email="agent-checkpoint@example.com",
    )
    created = await create_batch_fixture(
        client,
        headers=headers,
        episode=episode,
        version_id=script["version"]["id"],
        key="agent-checkpoint",
    )
    async with session_factory() as session:
        batch = await session.get(StoryboardDraftBatch, UUID(created["id"]))
        assert batch is not None
        assert batch.task_id is not None
        session.expunge(batch)
    yield batch


def _scene_context() -> SceneContext:
    return SceneContext(
        scene_key=1,
        scene_id=uuid7(),
        target_duration_ms=4_000,
        aspect_ratio="9:16",
        units=(
            SceneContextUnit(
                unit_version_id=uuid7(),
                position=1,
                kind="action",
                exact_text="沈岚拉下总闸。",
                required_for_coverage=True,
            ),
        ),
    )


def _checkpoint(
    batch: StoryboardDraftBatch,
    **updates: Any,
) -> StoryboardCheckpoint:
    assert batch.task_id is not None
    value: dict[str, Any] = {
        "batch_id": batch.id,
        "task_id": batch.task_id,
        "harness_version": STORYBOARD_AGENT_HARNESS_VERSION,
        "input_hash": batch.input_hash,
        "stage": "contexts_built",
        "stage_attempt": 1,
        "status": "running",
        "repair_round": 0,
        "scene_contexts": (_scene_context(),),
    }
    value.update(updates)
    return StoryboardCheckpoint.model_validate(value)


async def _activate_run_lease(
    session_factory: async_sessionmaker[AsyncSession],
    draft_batch: StoryboardDraftBatch,
) -> UUID:
    run_token = uuid7()
    async with session_factory() as session, session.begin():
        persisted = await session.scalar(
            select(StoryboardDraftBatch)
            .where(StoryboardDraftBatch.id == draft_batch.id)
            .with_for_update()
        )
        assert persisted is not None
        persisted.status = "running"
        persisted.agent_run_token = run_token
        persisted.agent_lease_expires_at = datetime.now(UTC) + timedelta(minutes=5)
    return run_token


def _terminal_checkpoint(batch: StoryboardDraftBatch) -> StoryboardCheckpoint:
    assert batch.task_id is not None
    context = _scene_context()
    result = StoryboardProviderResult.model_validate(
        {
            "shots": [
                {
                    "proposal_key": "main",
                    "position": 1,
                    "title": "拉下总闸",
                    "unit_positions": [1],
                    "dialogue_unit_positions": [],
                    "purpose": "交代断电动作",
                    "continuity_note": "保持人物朝向控制台",
                    "shot_size": "medium",
                    "camera_angle": "eye_level",
                    "camera_movement": "static",
                    "composition": "人物与总闸同框",
                    "environment": "控制室",
                    "subject_placements": [],
                    "mood_lighting": "冷白顶光",
                    "action_beats": [
                        {
                            "beat_key": "switch",
                            "order": 1,
                            "description": "沈岚拉下总闸",
                        }
                    ],
                    "duration_ms": 4_000,
                    "ambient": "电流声",
                    "asset_bindings": [],
                    "first_frame": "手停在抬起的总闸旁",
                    "last_frame": "总闸落下",
                    "risk_codes": ["review.reaction_subtle"],
                }
            ]
        }
    )
    draft = SceneDraft(scene_key=1, result=result)
    return StoryboardCheckpoint(
        batch_id=batch.id,
        task_id=batch.task_id,
        harness_version=STORYBOARD_AGENT_HARNESS_VERSION,
        input_hash=batch.input_hash,
        stage="final_gate_passed",
        stage_attempt=1,
        status="completed",
        repair_round=0,
        scene_contexts=(context,),
        scene_drafts=(draft,),
        assembled=assemble_storyboard((context,), (draft,)),
    )


@pytest.mark.asyncio
async def test_checkpoint_store_round_trips_and_versions_batch_scoped_json(
    session_factory: async_sessionmaker[AsyncSession],
    draft_batch: StoryboardDraftBatch,
) -> None:
    store = DatabaseStoryboardCheckpointStore(session_factory)
    run_token = await _activate_run_lease(session_factory, draft_batch)
    first = _checkpoint(draft_batch, run_token=run_token)

    await store.save(first)
    loaded = await store.load_latest(draft_batch.id, draft_batch.input_hash)

    assert loaded == first
    async with session_factory() as session:
        persisted = await session.get(StoryboardDraftBatch, draft_batch.id)
        assert persisted is not None
        assert persisted.agent_checkpoint == first.model_dump(mode="json")
        assert persisted.agent_checkpoint_revision == 1
        first_updated_at = persisted.agent_checkpoint_updated_at
        assert isinstance(first_updated_at, datetime)

    second = first.model_copy(update={"stage_attempt": 2})
    await store.save(second)

    async with session_factory() as session:
        persisted = await session.get(StoryboardDraftBatch, draft_batch.id)
        assert persisted is not None
        assert persisted.agent_checkpoint == second.model_dump(mode="json")
        assert persisted.agent_checkpoint_revision == 2
        assert persisted.agent_checkpoint_updated_at is not None
        assert persisted.agent_checkpoint_updated_at >= first_updated_at


@pytest.mark.asyncio
async def test_checkpoint_store_rejects_identity_mismatches_without_overwriting(
    session_factory: async_sessionmaker[AsyncSession],
    draft_batch: StoryboardDraftBatch,
) -> None:
    store = DatabaseStoryboardCheckpointStore(session_factory)
    run_token = await _activate_run_lease(session_factory, draft_batch)
    valid = _checkpoint(draft_batch, run_token=run_token)
    await store.save(valid)

    wrong_task = valid.model_copy(update={"task_id": uuid7()})
    with pytest.raises(StoryboardCheckpointMismatchError, match="task_id"):
        await store.save(wrong_task)

    wrong_input = valid.model_copy(update={"input_hash": "f" * 64})
    with pytest.raises(StoryboardCheckpointMismatchError, match="input_hash"):
        await store.save(wrong_input)

    wrong_run = valid.model_copy(update={"run_token": uuid7()})
    with pytest.raises(StoryboardCheckpointMismatchError, match="run_token"):
        await store.save(wrong_run)

    async with session_factory() as session, session.begin():
        persisted = await session.get(StoryboardDraftBatch, draft_batch.id)
        assert persisted is not None
        persisted.agent_lease_expires_at = datetime.now(UTC) - timedelta(seconds=1)
    with pytest.raises(StoryboardCheckpointMismatchError, match="expired"):
        await store.save(valid)

    assert await store.load_latest(draft_batch.id, "f" * 64) is None
    assert await store.load_latest(uuid7(), draft_batch.input_hash) is None
    assert await store.load_latest(draft_batch.id, draft_batch.input_hash) == valid


@pytest.mark.asyncio
async def test_checkpoint_store_fails_closed_for_corrupt_old_or_forged_payloads(
    session_factory: async_sessionmaker[AsyncSession],
    draft_batch: StoryboardDraftBatch,
) -> None:
    store = DatabaseStoryboardCheckpointStore(session_factory)
    valid = _checkpoint(draft_batch)

    payloads = (
        {**valid.model_dump(mode="json"), "harness_version": "storyboard-agent-harness-v0"},
        {**valid.model_dump(mode="json"), "task_id": str(uuid7())},
        {"stage": "contexts_built", "unexpected": "broken"},
    )
    for revision, payload in enumerate(payloads, start=1):
        async with session_factory() as session, session.begin():
            persisted = await session.get(StoryboardDraftBatch, draft_batch.id)
            assert persisted is not None
            persisted.agent_checkpoint = payload
            persisted.agent_checkpoint_revision = revision
            persisted.agent_checkpoint_updated_at = datetime.now().astimezone()

        assert await store.load_latest(draft_batch.id, draft_batch.input_hash) is None


@pytest.mark.asyncio
async def test_checkpoint_store_fails_closed_for_inconsistent_terminal_payload(
    session_factory: async_sessionmaker[AsyncSession],
    draft_batch: StoryboardDraftBatch,
) -> None:
    store = DatabaseStoryboardCheckpointStore(session_factory)
    run_token = await _activate_run_lease(session_factory, draft_batch)
    valid = _terminal_checkpoint(draft_batch).model_copy(update={"run_token": run_token})
    await store.save(valid)
    assert await store.load_latest(draft_batch.id, draft_batch.input_hash) == valid

    assembled = valid.assembled
    assert assembled is not None
    inconsistent = valid.model_copy(
        update={"assembled": assembled.model_copy(update={"result_hash": "f" * 64})}
    )
    with pytest.raises(
        StoryboardCheckpointMismatchError,
        match="internally inconsistent",
    ):
        await store.save(inconsistent)

    payload = valid.model_dump(mode="json")
    assembled_payload = payload["assembled"]
    assert isinstance(assembled_payload, dict)
    assembled_payload["total_duration_ms"] = 4_500
    async with session_factory() as session, session.begin():
        persisted = await session.get(StoryboardDraftBatch, draft_batch.id)
        assert persisted is not None
        persisted.agent_checkpoint = payload
        persisted.agent_checkpoint_revision += 1
        persisted.agent_checkpoint_updated_at = datetime.now().astimezone()

    assert await store.load_latest(draft_batch.id, draft_batch.input_hash) is None
