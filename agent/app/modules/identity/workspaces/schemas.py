from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class WorkspaceResponse(BaseModel):
    id: UUID
    name: str
    status: Literal["active", "archived"]
    role: Literal["owner", "editor", "viewer"]
    revision: int


class WorkspaceCreateRequest(CommandModel):
    name: str = Field(min_length=1, max_length=120)


class WorkspaceUpdateRequest(CommandModel):
    name: str = Field(min_length=1, max_length=120)
    expected_revision: int = Field(ge=1)


class WorkspaceStateRequest(CommandModel):
    expected_revision: int = Field(ge=1)
