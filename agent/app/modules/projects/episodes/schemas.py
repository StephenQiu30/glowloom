from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class EpisodeCreateRequest(CommandModel):
    name: str = Field(min_length=1, max_length=120)
    target_duration_ms: int = Field(default=90000, ge=1000, le=7_200_000)


class EpisodeUpdateRequest(CommandModel):
    name: str | None = Field(default=None, min_length=1, max_length=120)
    target_duration_ms: int | None = Field(default=None, ge=1000, le=7_200_000)
    expected_revision: int = Field(ge=1)


class EpisodeStateRequest(CommandModel):
    expected_revision: int = Field(ge=1)


class EpisodeReorderRequest(CommandModel):
    episode_ids: list[UUID] = Field(min_length=1)
    expected_revision: int = Field(ge=1)


class EpisodeResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    project_id: UUID
    name: str
    position: int
    target_duration_ms: int
    status: Literal["active", "archived"]
    revision: int
    current_script_version_id: UUID | None
    current_timeline_version_id: UUID | None


class EpisodeOrderResponse(BaseModel):
    items: list[EpisodeResponse]
    project_revision: int
