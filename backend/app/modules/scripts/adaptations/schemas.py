from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator

from app.modules.scripts.versions.schemas import (
    CurrentScriptVersionResponse,
    ScriptVersionResponse,
)


def _normalize_body(value: object) -> object:
    if isinstance(value, str):
        return value.replace("\r\n", "\n").replace("\r", "\n")
    return value


def _reject_blank_body(value: str) -> str:
    if not value.strip():
        raise ValueError("script body must contain text")
    return value


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class AdaptationRunCreateRequest(CommandModel):
    input_script_version_id: UUID
    target_duration_ms: int = Field(ge=15_000, le=600_000)
    core_plot_points: list[str] = Field(min_length=1, max_length=12)
    pacing: Literal["slow", "balanced", "fast"]
    colloquial_dialogue: bool
    idempotency_key: str = Field(min_length=1, max_length=200)

    @field_validator("core_plot_points")
    @classmethod
    def normalize_plot_points(cls, value: list[str]) -> list[str]:
        normalized = [item.strip() for item in value]
        if any(not item or len(item) > 200 for item in normalized):
            raise ValueError("core plot points must contain 1-200 characters")
        return normalized


class AdaptationDraftUpdateRequest(CommandModel):
    body: str = Field(min_length=1, max_length=20_000)
    expected_revision: int = Field(ge=1)

    _normalize_newlines = field_validator("body", mode="before")(_normalize_body)
    _reject_blank = field_validator("body")(_reject_blank_body)


class AdaptationPublishRequest(CommandModel):
    expected_run_revision: int = Field(ge=1)
    expected_current_version_id: UUID
    idempotency_key: str = Field(min_length=1, max_length=200)


class AdaptationCancelRequest(CommandModel):
    expected_revision: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)


class AdaptationConstraintsResponse(BaseModel):
    target_duration_ms: int
    core_plot_points: list[str]
    pacing: Literal["slow", "balanced", "fast"]
    colloquial_dialogue: bool


class AdaptationRunResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    episode_id: UUID
    source_id: UUID
    input_script_version_id: UUID
    input_hash: str
    constraints: AdaptationConstraintsResponse
    status: Literal["queued", "running", "succeeded", "published", "failed", "cancelled", "unknown"]
    revision: int
    task_id: UUID | None
    candidate_body: str | None
    candidate_hash: str | None
    draft_body: str | None
    draft_hash: str | None
    change_summary: str | None
    estimated_duration_ms: int | None
    error_code: str | None
    published_script_version_id: UUID | None
    created_at: datetime
    updated_at: datetime


class AdaptationDiffResponse(BaseModel):
    base_version_id: UUID
    adaptation_run_id: UUID
    added_lines: int
    removed_lines: int
    diff_lines: list[str]


class AdaptationPublishResponse(BaseModel):
    run: AdaptationRunResponse
    version: ScriptVersionResponse
    current: CurrentScriptVersionResponse


class ScriptAdaptationProviderResult(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)

    adapted_script_text: str = Field(min_length=1, max_length=20_000)
    change_summary: str = Field(min_length=1, max_length=1000)
    estimated_duration_ms: int = Field(ge=1000, le=600_000)

    _normalize_newlines = field_validator("adapted_script_text", mode="before")(_normalize_body)
    _reject_blank = field_validator("adapted_script_text")(_reject_blank_body)

    @field_validator("change_summary")
    @classmethod
    def normalize_summary(cls, value: str) -> str:
        normalized = value.strip()
        if not normalized:
            raise ValueError("change summary must contain text")
        return normalized
