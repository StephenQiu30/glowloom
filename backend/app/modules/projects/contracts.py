from dataclasses import dataclass
from typing import Literal
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


@dataclass(frozen=True, slots=True)
class EpisodeContentContext:
    episode_id: UUID
    workspace_id: UUID
    current_script_version_id: UUID | None
    revision: int
