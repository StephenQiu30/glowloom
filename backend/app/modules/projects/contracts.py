from dataclasses import dataclass
from decimal import Decimal
from typing import Literal, Protocol
from uuid import UUID

from pydantic import BaseModel


class DeleteBlocker(BaseModel):
    code: str
    resource_type: str
    resource_id: UUID
    summary: str


class DeletePreflightResponse(BaseModel):
    allowed: bool
    blockers: list[DeleteBlocker]


class DeleteResponse(BaseModel):
    deleted: Literal[True] = True


class EpisodeScriptVersionCountReader(Protocol):
    async def __call__(
        self,
        *,
        workspace_id: UUID,
        episode_ids: list[UUID],
    ) -> dict[UUID, int]: ...


@dataclass(frozen=True, slots=True)
class EpisodeStoryboardReferenceSummary:
    shot_count: int
    spec_version_count: int


class EpisodeStoryboardReferenceReader(Protocol):
    async def __call__(
        self,
        *,
        workspace_id: UUID,
        episode_ids: list[UUID],
    ) -> dict[UUID, EpisodeStoryboardReferenceSummary]: ...


@dataclass(frozen=True, slots=True)
class ProjectAssetReferenceSummary:
    asset_count: int
    version_count: int


class ProjectAssetReferenceReader(Protocol):
    async def __call__(
        self,
        *,
        workspace_id: UUID,
        project_ids: list[UUID],
    ) -> dict[UUID, ProjectAssetReferenceSummary]: ...


@dataclass(frozen=True, slots=True)
class EpisodeContentContext:
    episode_id: UUID
    workspace_id: UUID
    project_id: UUID
    current_script_version_id: UUID | None
    revision: int


@dataclass(frozen=True, slots=True)
class ProjectContentContext:
    project_id: UUID
    workspace_id: UUID
    status: Literal["active", "archived"]
    revision: int


@dataclass(frozen=True, slots=True)
class GenerationProjectContext:
    project_id: UUID
    episode_id: UUID | None
    workspace_id: UUID
    project_status: Literal["active", "archived"]
    episode_status: Literal["active", "archived"] | None
    budget_limit: Decimal
    currency: str
    project_revision: int
    episode_revision: int | None
