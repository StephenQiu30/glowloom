from __future__ import annotations

from typing import Any, Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field

SCHEMA_VERSION = "agent-candidate-v1"


class Invocation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    invocation_id: UUID
    kind: Literal["production_bible", "script_structure", "storyboard_draft"]
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    schema_version: Literal["agent-candidate-v1"]
    payload: dict[str, Any]


class Executor(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: str
    version: str
    model: str


class ResultError(BaseModel):
    model_config = ConfigDict(extra="forbid")

    code: str
    summary: str
    retryable: bool


class Result(BaseModel):
    model_config = ConfigDict(extra="forbid")

    invocation_id: UUID
    kind: Literal["production_bible", "script_structure", "storyboard_draft"]
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    status: Literal["succeeded", "failed", "unknown"]
    schema_version: Literal["agent-candidate-v1"] = SCHEMA_VERSION
    candidate: dict[str, Any] | None
    result_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    executor: Executor
    error: ResultError | None
