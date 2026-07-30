from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class ScriptImportRequest(CommandModel):
    input_type: Literal["text"]
    title: str = Field(min_length=1, max_length=120)
    body: str = Field(min_length=1, max_length=20_000)
    rights_declaration: str = Field(min_length=1, max_length=1000)
    idempotency_key: str = Field(min_length=1, max_length=200)

    @field_validator("body", mode="before")
    @classmethod
    def normalize_newlines(cls, value: object) -> object:
        if isinstance(value, str):
            return value.replace("\r\n", "\n").replace("\r", "\n")
        return value

    @field_validator("body")
    @classmethod
    def reject_blank_body(cls, value: str) -> str:
        if not value.strip():
            raise ValueError("script body must contain text")
        return value


class ScriptVersionPublishRequest(CommandModel):
    body: str = Field(min_length=1, max_length=20_000)
    expected_current_version_id: UUID | None

    @field_validator("body", mode="before")
    @classmethod
    def normalize_newlines(cls, value: object) -> object:
        if isinstance(value, str):
            return value.replace("\r\n", "\n").replace("\r", "\n")
        return value

    @field_validator("body")
    @classmethod
    def reject_blank_body(cls, value: str) -> str:
        if not value.strip():
            raise ValueError("script body must contain text")
        return value


class ScriptSourceResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    episode_id: UUID
    input_type: Literal["text", "media"]
    title: str
    source_media_version_id: UUID | None
    rights_declaration: str
    status: Literal["active", "archived"]
    revision: int
    created_at: datetime


class ScriptVersionResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    source_id: UUID
    version_no: int
    status: Literal["draft", "published"]
    body: str
    content_hash: str
    created_by: UUID
    created_at: datetime


class ScriptImportResponse(BaseModel):
    source: ScriptSourceResponse
    version: ScriptVersionResponse


class CurrentScriptVersionRequest(CommandModel):
    version_id: UUID
    expected_current_version_id: UUID | None


class CurrentScriptVersionResponse(BaseModel):
    episode_id: UUID
    current_script_version_id: UUID
    episode_revision: int


class ScriptVersionPublishResponse(BaseModel):
    version: ScriptVersionResponse
    current: CurrentScriptVersionResponse


class ScriptSourceStateRequest(CommandModel):
    expected_revision: int = Field(ge=1)


class ScriptVersionDiffResponse(BaseModel):
    base_version_id: UUID
    target_version_id: UUID
    added_lines: int
    removed_lines: int
    diff_lines: list[str]


class PaginatedScriptVersions(BaseModel):
    items: list[ScriptVersionResponse]
    total: int
    limit: int
    offset: int
