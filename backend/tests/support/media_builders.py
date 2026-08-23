from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.media.models import MediaLocation, MediaObject, MediaVersion


async def seed_ready_media_version(
    session_factory: async_sessionmaker[AsyncSession],
    *,
    workspace_id: UUID,
    actor_id: UUID,
    kind: str,
    filename: str,
    mime_type: str,
) -> UUID:
    object_id = uuid7()
    version_id = uuid7()
    async with session_factory() as session, session.begin():
        session.add(
            MediaObject(
                id=object_id,
                workspace_id=workspace_id,
                kind=kind,
                source_type="upload",
                status="active",
                current_version_id=version_id,
                revision=1,
            )
        )
        session.add(
            MediaVersion(
                id=version_id,
                workspace_id=workspace_id,
                media_object_id=object_id,
                version_no=1,
                filename=filename,
                sha256="a" * 64,
                size_bytes=64,
                mime_type=mime_type,
                probe_status="ready",
                probe_attempt=1,
                width=1 if kind == "image" else None,
                height=1 if kind == "image" else None,
                duration_ms=1000 if kind == "audio" else None,
                created_by=actor_id,
            )
        )
        session.add(
            MediaLocation(
                workspace_id=workspace_id,
                media_version_id=version_id,
                storage_profile="test-private",
                bucket="lanverse-test",
                object_key=f"assets/{version_id}/{filename}",
                status="active",
                verified_at=datetime.now(UTC),
            )
        )
    return version_id
