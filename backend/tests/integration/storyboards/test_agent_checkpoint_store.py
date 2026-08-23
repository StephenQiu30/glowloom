from collections.abc import AsyncIterator
from datetime import datetime
from typing import Any
from uuid import UUID

import httpx
import pytest
import pytest_asyncio
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.storyboards.agents import (
    STORYBOARD_AGENT_HARNESS_VERSION,
    DatabaseStoryboardCheckpointStore,
    SceneContext,
    SceneContextUnit,
    StoryboardCheckpoint,
    StoryboardCheckpointMismatchError,
)
from app.modules.storyboards.drafts.models import StoryboardDraftBatch
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


def _checkpoint(
    batch: StoryboardDraftBatch,
    **updates: Any,
) -> StoryboardCheckpoint:
    assert batch.task_id is not None
    context = SceneContext(
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
    value: dict[str, Any] = {
        "batch_id": batch.id,
        "task_id": batch.task_id,
        "harness_version": STORYBOARD_AGENT_HARNESS_VERSION,
        "input_hash": batch.input_hash,
        "stage": "contexts_built",
        "stage_attempt": 1,
        "status": "running",
        "repair_round": 0,
        "scene_contexts": (context,),
    }
    value.update(updates)
    return StoryboardCheckpoint.model_validate(value)


@pytest.mark.asyncio
async def test_checkpoint_store_round_trips_and_versions_batch_scoped_json(
    session_factory: async_sessionmaker[AsyncSession],
    draft_batch: StoryboardDraftBatch,
) -> None:
    store = DatabaseStoryboardCheckpointStore(session_factory)
    first = _checkpoint(draft_batch)

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
    valid = _checkpoint(draft_batch)
    await store.save(valid)

    wrong_task = valid.model_copy(update={"task_id": uuid7()})
    with pytest.raises(StoryboardCheckpointMismatchError, match="task_id"):
        await store.save(wrong_task)

    wrong_input = valid.model_copy(update={"input_hash": "f" * 64})
    with pytest.raises(StoryboardCheckpointMismatchError, match="input_hash"):
        await store.save(wrong_input)

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
