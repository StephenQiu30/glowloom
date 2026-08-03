from datetime import datetime
from typing import Literal
from uuid import UUID

from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.media.models import MediaLocation, MediaObject, MediaVersion, UploadSession


async def find_upload_session(
    session: AsyncSession,
    upload_session_id: UUID,
    *,
    for_update: bool = False,
) -> UploadSession | None:
    query = select(UploadSession).where(UploadSession.id == upload_session_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_idempotent_upload(
    session: AsyncSession, workspace_id: UUID, idempotency_key: str
) -> UploadSession | None:
    return await session.scalar(
        select(UploadSession).where(
            UploadSession.workspace_id == workspace_id,
            UploadSession.idempotency_key == idempotency_key,
        )
    )


async def find_media_object(
    session: AsyncSession,
    media_object_id: UUID,
    *,
    for_update: bool = False,
) -> MediaObject | None:
    query = select(MediaObject).where(MediaObject.id == media_object_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_media_version(
    session: AsyncSession,
    version_id: UUID,
    *,
    for_update: bool = False,
) -> tuple[MediaVersion, MediaObject] | None:
    query = (
        select(MediaVersion, MediaObject)
        .join(
            MediaObject,
            (MediaObject.id == MediaVersion.media_object_id)
            & (MediaObject.workspace_id == MediaVersion.workspace_id),
        )
        .where(MediaVersion.id == version_id)
    )
    if for_update:
        query = query.with_for_update(of=(MediaVersion, MediaObject))
    row = (await session.execute(query)).one_or_none()
    return None if row is None else (row[0], row[1])


async def find_media_versions_with_active_locations(
    session: AsyncSession,
    version_ids: list[UUID],
) -> list[tuple[MediaVersion, MediaObject, MediaLocation | None]]:
    if not version_ids:
        return []
    rows = await session.execute(
        select(MediaVersion, MediaObject, MediaLocation)
        .join(
            MediaObject,
            (MediaObject.id == MediaVersion.media_object_id)
            & (MediaObject.workspace_id == MediaVersion.workspace_id),
        )
        .outerjoin(
            MediaLocation,
            (MediaLocation.media_version_id == MediaVersion.id)
            & (MediaLocation.workspace_id == MediaVersion.workspace_id)
            & (MediaLocation.status == "active"),
        )
        .where(MediaVersion.id.in_(version_ids))
    )
    return [(row[0], row[1], row[2]) for row in rows]


async def find_active_location(
    session: AsyncSession, version_id: UUID
) -> MediaLocation | None:
    return await session.scalar(
        select(MediaLocation).where(
            MediaLocation.media_version_id == version_id,
            MediaLocation.status == "active",
        )
    )


async def list_media_versions(
    session: AsyncSession,
    workspace_id: UUID,
    *,
    kind: str | None,
    source_type: Literal["upload", "generated", "rendered"] | None,
    include_archived: bool,
    created_from: datetime | None,
    created_to: datetime | None,
    limit: int,
    offset: int,
) -> tuple[list[tuple[MediaVersion, MediaObject]], int]:
    filters = [MediaVersion.workspace_id == workspace_id]
    if kind is not None:
        filters.append(MediaObject.kind == kind)
    if source_type is not None:
        filters.append(MediaObject.source_type == source_type)
    if not include_archived:
        filters.append(MediaObject.status == "active")
    if created_from is not None:
        filters.append(MediaVersion.created_at >= created_from)
    if created_to is not None:
        filters.append(MediaVersion.created_at <= created_to)
    base = (
        select(MediaVersion, MediaObject)
        .join(
            MediaObject,
            (MediaObject.id == MediaVersion.media_object_id)
            & (MediaObject.workspace_id == MediaVersion.workspace_id),
        )
        .where(*filters)
    )
    total = await session.scalar(
        select(func.count()).select_from(base.order_by(None).subquery())
    )
    rows = await session.execute(
        base.order_by(MediaVersion.created_at.desc(), MediaVersion.version_no.desc())
        .limit(limit)
        .offset(offset)
    )
    return [(row[0], row[1]) for row in rows], total or 0
