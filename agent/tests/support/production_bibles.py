from datetime import UTC, datetime
from hashlib import sha256
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.scripts.production_bibles.models import ProductionBible


async def seed_confirmed_production_bible(
    factory: async_sessionmaker[AsyncSession],
    *,
    workspace_id: UUID,
    project_id: UUID,
    document_revision_id: UUID,
    input_hash: str,
    actor_id: UUID,
) -> UUID:
    bible_id = uuid7()
    now = datetime.now(UTC)
    result_hash = sha256(f"confirmed:{bible_id}".encode()).hexdigest()
    async with factory() as session, session.begin():
        session.add(
            ProductionBible(
                id=bible_id,
                workspace_id=workspace_id,
                project_id=project_id,
                document_revision_id=document_revision_id,
                task_id=None,
                status="confirmed",
                input_hash=input_hash,
                result_hash=result_hash,
                engine_version="test-confirmed-bible",
                model_name="test",
                prompt_version="test",
                schema_version="test",
                harness_version="test",
                checkpoint=None,
                checkpoint_revision=0,
                checkpoint_updated_at=None,
                run_token=None,
                lease_expires_at=None,
                review_issues=[],
                revision=3,
                idempotency_key=f"test-bible:{bible_id}",
                confirm_idempotency_key=f"test-confirm:{bible_id}",
                confirm_command_hash=sha256(f"command:{bible_id}".encode()).hexdigest(),
                confirm_result={
                    "bible_id": str(bible_id),
                    "status": "confirmed",
                    "revision": 3,
                    "entity_asset_ids": {},
                    "state_bindings": {},
                },
                confirmed_at=now,
                confirmed_by=actor_id,
                created_by=actor_id,
                created_at=now,
                updated_at=now,
            )
        )
    return bible_id
