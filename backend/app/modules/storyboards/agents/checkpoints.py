from datetime import UTC, datetime
from uuid import UUID

from pydantic import ValidationError
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.storyboards.drafts.models import StoryboardDraftBatch

from .harness import STORYBOARD_AGENT_HARNESS_VERSION
from .schemas import StoryboardCheckpoint


class StoryboardCheckpointMismatchError(ValueError):
    """Raised when a checkpoint does not belong to its target draft batch."""


class DatabaseStoryboardCheckpointStore:
    """Persists the latest agent checkpoint on its storyboard draft batch."""

    def __init__(
        self,
        session_factory: async_sessionmaker[AsyncSession],
    ) -> None:
        self._session_factory = session_factory

    async def load_latest(
        self,
        batch_id: UUID,
        input_hash: str,
    ) -> StoryboardCheckpoint | None:
        async with self._session_factory() as session, session.begin():
            batch = await session.scalar(
                select(StoryboardDraftBatch)
                .where(StoryboardDraftBatch.id == batch_id)
                .with_for_update()
            )
            if (
                batch is None
                or batch.task_id is None
                or batch.input_hash != input_hash
                or batch.agent_checkpoint is None
                or batch.agent_checkpoint_revision < 1
                or batch.agent_checkpoint_updated_at is None
            ):
                return None

            try:
                checkpoint = StoryboardCheckpoint.model_validate(batch.agent_checkpoint)
            except (TypeError, ValidationError, ValueError):
                return None

            if (
                checkpoint.batch_id != batch.id
                or checkpoint.task_id != batch.task_id
                or checkpoint.input_hash != batch.input_hash
                or checkpoint.input_hash != input_hash
                or checkpoint.harness_version != STORYBOARD_AGENT_HARNESS_VERSION
            ):
                return None
            return checkpoint

    async def save(self, checkpoint: StoryboardCheckpoint) -> None:
        async with self._session_factory() as session, session.begin():
            batch = await session.scalar(
                select(StoryboardDraftBatch)
                .where(StoryboardDraftBatch.id == checkpoint.batch_id)
                .with_for_update()
            )
            if batch is None:
                raise StoryboardCheckpointMismatchError(
                    f"batch_id {checkpoint.batch_id} does not exist"
                )
            if batch.task_id != checkpoint.task_id:
                raise StoryboardCheckpointMismatchError(
                    "checkpoint task_id does not match the draft batch"
                )
            if batch.input_hash != checkpoint.input_hash:
                raise StoryboardCheckpointMismatchError(
                    "checkpoint input_hash does not match the draft batch"
                )
            if checkpoint.harness_version != STORYBOARD_AGENT_HARNESS_VERSION:
                raise StoryboardCheckpointMismatchError(
                    "checkpoint harness_version is not supported"
                )

            batch.agent_checkpoint = checkpoint.model_dump(mode="json")
            batch.agent_checkpoint_revision += 1
            batch.agent_checkpoint_updated_at = datetime.now(UTC)
