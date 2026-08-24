from datetime import UTC, datetime
from typing import cast
from uuid import UUID

from pydantic import ValidationError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.scripts.production_bibles import repository
from app.modules.scripts.production_bibles.harness import ProductionBibleCheckpoint
from app.modules.scripts.production_bibles.ports import PRODUCTION_BIBLE_HARNESS_VERSION


class ProductionBibleCheckpointMismatchError(ValueError):
    """Raised when a checkpoint no longer owns its Production Bible run."""


class DatabaseProductionBibleCheckpointStore:
    def __init__(
        self,
        session_factory: async_sessionmaker[AsyncSession],
    ) -> None:
        self._session_factory = session_factory

    async def load_latest(
        self,
        bible_id: UUID,
        input_hash: str,
    ) -> ProductionBibleCheckpoint | None:
        async with self._session_factory() as session, session.begin():
            bible = await repository.find_bible(session, bible_id, for_update=True)
            now = datetime.now(UTC)
            if (
                bible is None
                or bible.task_id is None
                or bible.status != "running"
                or bible.run_token is None
                or bible.lease_expires_at is None
                or bible.lease_expires_at <= now
                or bible.input_hash != input_hash
                or bible.checkpoint is None
                or bible.checkpoint_revision < 1
                or bible.checkpoint_updated_at is None
            ):
                return None
            try:
                checkpoint = ProductionBibleCheckpoint.model_validate(bible.checkpoint)
            except (TypeError, ValidationError, ValueError):
                return None
            if (
                checkpoint.bible_id != bible.id
                or checkpoint.input_hash != bible.input_hash
                or checkpoint.harness_version != bible.harness_version
                or checkpoint.harness_version != PRODUCTION_BIBLE_HARNESS_VERSION
            ):
                return None
            receipts = cast(dict[str, object], bible.resume_receipts)
            has_resume_receipt = any(
                isinstance(receipt, dict)
                and cast(dict[str, object], receipt).get("task_id") == str(bible.task_id)
                for receipt in receipts.values()
            )
            if checkpoint.task_id != bible.task_id and not has_resume_receipt:
                return None
            return checkpoint.model_copy(
                update={
                    "task_id": bible.task_id,
                    "run_token": bible.run_token,
                }
            )

    async def save(self, checkpoint: ProductionBibleCheckpoint) -> None:
        try:
            checkpoint = ProductionBibleCheckpoint.model_validate(
                checkpoint.model_dump(mode="json")
            )
        except (TypeError, ValidationError, ValueError) as error:
            raise ProductionBibleCheckpointMismatchError(
                "checkpoint payload is internally inconsistent"
            ) from error
        async with self._session_factory() as session, session.begin():
            bible = await repository.find_bible(
                session,
                checkpoint.bible_id,
                for_update=True,
            )
            now = datetime.now(UTC)
            if bible is None:
                raise ProductionBibleCheckpointMismatchError(
                    f"bible_id {checkpoint.bible_id} does not exist"
                )
            if bible.task_id != checkpoint.task_id:
                raise ProductionBibleCheckpointMismatchError(
                    "checkpoint task_id does not match the Production Bible"
                )
            if bible.input_hash != checkpoint.input_hash:
                raise ProductionBibleCheckpointMismatchError(
                    "checkpoint input_hash does not match the Production Bible"
                )
            if checkpoint.harness_version != bible.harness_version:
                raise ProductionBibleCheckpointMismatchError(
                    "checkpoint harness_version is not supported"
                )
            if bible.status != "running":
                raise ProductionBibleCheckpointMismatchError(
                    "checkpoint target Production Bible is not running"
                )
            if bible.run_token != checkpoint.run_token:
                raise ProductionBibleCheckpointMismatchError(
                    "checkpoint run token no longer owns the Production Bible lease"
                )
            if bible.lease_expires_at is None or bible.lease_expires_at <= now:
                raise ProductionBibleCheckpointMismatchError(
                    "checkpoint Production Bible lease has expired"
                )
            bible.checkpoint = checkpoint.model_dump(mode="json")
            bible.checkpoint_revision += 1
            bible.checkpoint_updated_at = now
            bible.updated_at = now
