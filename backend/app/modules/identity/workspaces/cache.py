from typing import Annotated, Literal, cast
from uuid import UUID

from fastapi import Depends, Request
from pydantic import BaseModel, ConfigDict, ValidationError

from app.core.config import Settings
from app.modules.caching import (
    CacheKey,
    CacheNamespace,
    CachePort,
    CacheUnavailableError,
    get_cache_port,
)
from app.modules.identity.models import Workspace


class WorkspaceProjection(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    workspace_id: UUID
    name: str
    status: Literal["active", "archived"]
    revision: int


class WorkspaceDetailCache:
    def __init__(self, port: CachePort, *, ttl_seconds: int, jitter_seconds: int) -> None:
        self._port = port
        self._ttl_seconds = ttl_seconds
        self._jitter_seconds = jitter_seconds

    @staticmethod
    def _revision_key(workspace_id: UUID) -> CacheKey:
        return CacheKey(
            namespace=CacheNamespace.WORKSPACE_DETAIL,
            scope=str(workspace_id),
            identity="revision",
            revision="current",
        )

    @staticmethod
    def _projection_key(workspace_id: UUID, revision: int) -> CacheKey:
        return CacheKey(
            namespace=CacheNamespace.WORKSPACE_DETAIL,
            scope=str(workspace_id),
            identity="workspace",
            revision=f"r{revision}",
        )

    async def get(self, workspace_id: UUID) -> WorkspaceProjection | None:
        revision_key = self._revision_key(workspace_id)
        try:
            raw_revision = await self._port.get(revision_key)
            if raw_revision is None:
                return None
            revision = int(raw_revision.decode("ascii"))
            if revision < 1:
                raise ValueError("workspace cache revision is invalid")
            raw_projection = await self._port.get(self._projection_key(workspace_id, revision))
            if raw_projection is None:
                await self._best_effort_delete(revision_key)
                return None
            projection = WorkspaceProjection.model_validate_json(raw_projection)
            if projection.workspace_id != workspace_id or projection.revision != revision:
                await self._best_effort_delete(revision_key)
                return None
            return projection
        except (
            CacheUnavailableError,
            UnicodeDecodeError,
            ValueError,
            ValidationError,
        ):
            return None

    async def store(self, workspace: Workspace) -> None:
        projection = WorkspaceProjection(
            workspace_id=workspace.id,
            name=workspace.name,
            status=cast(Literal["active", "archived"], workspace.status),
            revision=workspace.revision,
        )
        try:
            await self._port.set(
                self._projection_key(workspace.id, workspace.revision),
                projection.model_dump_json().encode("utf-8"),
                ttl_seconds=self._ttl_seconds,
                jitter_seconds=self._jitter_seconds,
            )
            await self._port.set_revision(
                self._revision_key(workspace.id),
                workspace.revision,
                ttl_seconds=self._ttl_seconds,
                jitter_seconds=self._jitter_seconds,
            )
        except CacheUnavailableError:
            return

    async def invalidate(self, workspace_id: UUID) -> None:
        await self._best_effort_delete(self._revision_key(workspace_id))

    async def _best_effort_delete(self, key: CacheKey) -> None:
        try:
            await self._port.delete(key)
        except CacheUnavailableError:
            return


def get_workspace_detail_cache(
    request: Request,
    port: Annotated[CachePort, Depends(get_cache_port)],
) -> WorkspaceDetailCache:
    settings: Settings = request.app.state.settings
    return WorkspaceDetailCache(
        port,
        ttl_seconds=settings.workspace_cache_ttl_seconds,
        jitter_seconds=settings.cache_ttl_jitter_seconds,
    )
