from dataclasses import dataclass
from decimal import Decimal
from typing import Literal, Protocol
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


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
class StoryboardEpisodeContext:
    episode_id: UUID
    project_id: UUID
    workspace_id: UUID
    current_script_version_id: UUID | None
    episode_revision: int
    project_revision: int
    target_duration_ms: int
    aspect_ratio: Literal["9:16", "16:9", "1:1"]
    visual_style: str | None


@dataclass(frozen=True, slots=True)
class ProjectContentContext:
    project_id: UUID
    workspace_id: UUID
    status: Literal["active", "archived"]
    revision: int


class EpisodeBatchItem(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    client_reference_id: UUID
    name: str = Field(min_length=1, max_length=120)
    target_duration_ms: int = Field(ge=1000, le=7_200_000)


class EpisodeBatchMaterializeCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    project_id: UUID
    expected_project_revision: int = Field(ge=1)
    expected_active_order_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    items: list[EpisodeBatchItem] = Field(min_length=1, max_length=10)


@dataclass(frozen=True, slots=True)
class MaterializedEpisodeReference:
    client_reference_id: UUID
    episode_id: UUID
    revision: int
    position: int
    current_script_version_id: UUID | None


@dataclass(frozen=True, slots=True)
class EpisodeBatchMaterializeResult:
    project_revision: int
    active_order_hash: str
    items: tuple[MaterializedEpisodeReference, ...]


class EpisodeScriptPublishItem(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    episode_id: UUID
    expected_revision: int = Field(ge=1)
    expected_current_script_version_id: UUID | None
    script_version_id: UUID


class EpisodeScriptPublishBatchCommand(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    project_id: UUID
    expected_project_revision: int = Field(ge=1)
    items: list[EpisodeScriptPublishItem] = Field(min_length=1, max_length=10)


@dataclass(frozen=True, slots=True)
class PublishedEpisodeScriptReference:
    episode_id: UUID
    revision: int
    previous_script_version_id: UUID | None
    current_script_version_id: UUID


@dataclass(frozen=True, slots=True)
class EpisodeScriptPublishBatchResult:
    project_revision: int
    items: tuple[PublishedEpisodeScriptReference, ...]


@dataclass(frozen=True, slots=True)
class ProjectEpisodeOrderSnapshot:
    project_id: UUID
    workspace_id: UUID
    project_revision: int
    active_episode_count: int
    active_order_hash: str


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
