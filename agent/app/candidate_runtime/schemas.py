from __future__ import annotations

from typing import Any, Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.candidate_runtime.canonical import canonical_hash

SCHEMA_VERSION = "agent-candidate-v1"


class ExecutionPolicy(BaseModel):
    model_config = ConfigDict(extra="forbid")

    definition_key: str
    definition_version: str
    prompt_version: str
    skill_bundle_version: str
    output_schema_version: str
    model_capability: str
    allowed_tools: list[str]
    max_model_calls: int = Field(ge=1)
    max_execution_seconds: int = Field(ge=1)

    def canonical_hash(self) -> str:
        return canonical_hash(self.model_dump(mode="json"))


def execution_policy_for(kind: str) -> ExecutionPolicy:
    if kind == "production_bible":
        return ExecutionPolicy(
            definition_key=kind,
            definition_version="production-bible-harness-v1",
            prompt_version="production-bible-prompt-v1",
            skill_bundle_version="production-bible-skills-v1",
            output_schema_version="production-bible-schema-v1",
            model_capability="structured_text",
            allowed_tools=[],
            max_model_calls=3,
            max_execution_seconds=900,
        )
    if kind == "storyboard_draft":
        return ExecutionPolicy(
            definition_key=kind,
            definition_version="storyboard-harness-v1",
            prompt_version="storyboard-prompt-v1",
            skill_bundle_version="storyboard-skills-v1",
            output_schema_version="storyboard-draft-schema-v1",
            model_capability="structured_text",
            allowed_tools=[],
            max_model_calls=1,
            max_execution_seconds=600,
        )
    raise ValueError("unsupported agent definition")


class Invocation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    invocation_id: UUID
    kind: Literal["production_bible", "storyboard_draft"]
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    schema_version: Literal["agent-candidate-v1"]
    execution_policy: ExecutionPolicy
    payload: dict[str, Any]

    @model_validator(mode="after")
    def validate_execution_policy(self) -> Invocation:
        manifest = execution_policy_for(self.kind)
        policy = self.execution_policy
        frozen_fields = (
            "definition_key",
            "definition_version",
            "prompt_version",
            "skill_bundle_version",
            "output_schema_version",
            "model_capability",
        )
        if (
            any(getattr(policy, field) != getattr(manifest, field) for field in frozen_fields)
            or policy.allowed_tools
            or policy.max_model_calls > manifest.max_model_calls
            or policy.max_execution_seconds > manifest.max_execution_seconds
        ):
            raise ValueError("agent execution policy is outside the definition manifest")
        return self


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
    kind: Literal["production_bible", "storyboard_draft"]
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    status: Literal["succeeded", "failed", "unknown"]
    schema_version: Literal["agent-candidate-v1"] = SCHEMA_VERSION
    candidate: dict[str, Any] | None
    result_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    executor: Executor
    error: ResultError | None
