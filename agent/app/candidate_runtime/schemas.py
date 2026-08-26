from __future__ import annotations

import hashlib
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


StoryGraphStage = Literal[
    "extract_source_evidence",
    "analyze_story",
    "reconcile_story",
    "segment_episodes",
    "analyze_episode",
    "reconcile_episode",
    "draft_storyboard",
    "detail_shots",
    "review_storygraph",
    "repair_candidate",
]


class StoryGraphExecutionPolicy(BaseModel):
    model_config = ConfigDict(extra="forbid")

    definition_key: Literal["storygraph_stage"]
    definition_version: str = Field(min_length=1)
    prompt_version: str = Field(min_length=1)
    skill_bundle_version: str = Field(min_length=1)
    skill_bundle_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    output_schema_version: str = Field(min_length=1)
    model_capability: Literal["structured_text"]
    codex_runtime_contract: Literal["codex-cli-ephemeral-read-only-v1"]
    allowed_tools: list[str]
    max_model_calls: int = Field(ge=1)
    max_execution_seconds: int = Field(ge=1)

    @model_validator(mode="after")
    def reject_tools(self) -> StoryGraphExecutionPolicy:
        if self.allowed_tools:
            raise ValueError("StoryGraph execution policy must not allow tools")
        return self

    def canonical_hash(self) -> str:
        return canonical_hash(self.model_dump(mode="json"))


class StoryGraphSourceRef(BaseModel):
    model_config = ConfigDict(extra="forbid")

    owner_kind: str = Field(min_length=1)
    owner_logical_id: str = Field(min_length=1)
    owner_version_id: UUID
    revision: int = Field(ge=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class StoryGraphUpstreamCandidateRef(BaseModel):
    model_config = ConfigDict(extra="forbid")

    stage: StoryGraphStage
    shard_key: str = Field(min_length=1)
    candidate_revision_id: UUID
    candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    source_invocation_id: UUID
    source_result_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class StoryGraphShardManifestRef(BaseModel):
    model_config = ConfigDict(extra="forbid")

    manifest_id: UUID
    version: int = Field(ge=1)
    hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class StoryGraphInvocationShard(BaseModel):
    model_config = ConfigDict(extra="forbid")

    kind: str = Field(min_length=1)
    key: str = Field(min_length=1)
    tree_path: str = Field(min_length=1)
    parent_key: str | None = None
    absolute_start: int | None = Field(default=None, ge=0)
    absolute_end: int | None = Field(default=None, ge=1)

    @model_validator(mode="after")
    def validate_range(self) -> StoryGraphInvocationShard:
        if (self.absolute_start is None) != (self.absolute_end is None):
            raise ValueError("StoryGraph shard range must be complete")
        if (
            self.absolute_start is not None
            and self.absolute_end is not None
            and self.absolute_end <= self.absolute_start
        ):
            raise ValueError("StoryGraph shard range must be increasing")
        return self


class StoryGraphStagePayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    stage: StoryGraphStage
    shard_key: str = Field(min_length=1)
    workspace_id: UUID
    project_id: UUID
    source_refs: list[StoryGraphSourceRef]
    base_storygraph_version_id: UUID | None = None
    base_storygraph_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    upstream_candidates: list[StoryGraphUpstreamCandidateRef]
    shard_manifest_ref: StoryGraphShardManifestRef
    shard: StoryGraphInvocationShard
    stage_input: dict[str, Any]

    @model_validator(mode="after")
    def validate_refs(self) -> StoryGraphStagePayload:
        if self.shard_key != self.shard.key:
            raise ValueError("StoryGraph shard key does not match payload")
        if (self.base_storygraph_version_id is None) != (self.base_storygraph_hash is None):
            raise ValueError("base StoryGraph reference must be complete")
        return self


class StoryGraphStageInvocation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    invocation_id: UUID
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-stage-wire-v1"]
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    execution_policy: StoryGraphExecutionPolicy
    payload: StoryGraphStagePayload

    @model_validator(mode="after")
    def validate_input_hash(self) -> StoryGraphStageInvocation:
        computed = self.compute_input_hash()
        if self.input_hash != computed:
            raise ValueError(
                f"StoryGraph input hash mismatch: got {self.input_hash} want {computed}"
            )
        return self

    def compute_input_hash(self) -> str:
        payload = self.payload.model_dump(mode="json", exclude_none=True)
        payload["source_refs"] = sorted(
            payload["source_refs"],
            key=lambda item: (
                item["owner_kind"],
                item["owner_logical_id"],
                item["owner_version_id"],
                item["revision"],
                item["content_hash"],
            ),
        )
        payload["upstream_candidates"] = sorted(
            payload["upstream_candidates"],
            key=lambda item: (
                item["stage"],
                item["shard_key"],
                item["candidate_revision_id"],
                item["candidate_revision_hash"],
            ),
        )
        return canonical_hash(
            {
                "wire_schema_version": self.wire_schema_version,
                "execution_policy": self.execution_policy.model_dump(mode="json"),
                "payload": payload,
            }
        )

    def stage_instance_key(self) -> str:
        material = (
            "storygraph-stage-v1"
            + self.payload.stage
            + self.payload.shard_key
            + self.payload.shard_manifest_ref.hash
            + self.input_hash
        )
        return hashlib.sha256(material.encode("utf-8")).hexdigest()


class StoryGraphStageIssue(BaseModel):
    model_config = ConfigDict(extra="forbid")

    code: str = Field(min_length=1)
    summary: str = Field(min_length=1)


class StoryGraphStageResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    invocation_id: UUID
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-stage-wire-v1"]
    stage: StoryGraphStage
    shard_key: str = Field(min_length=1)
    status: Literal["succeeded", "failed", "unknown"]
    candidate_type: str = Field(min_length=1)
    candidate: dict[str, Any] | None
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    result_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    issues: list[StoryGraphStageIssue]
    executor: Executor
    error: ResultError | None

    @model_validator(mode="after")
    def validate_status(self) -> StoryGraphStageResult:
        if self.status == "succeeded":
            if self.candidate is None or self.result_hash is None or self.error is not None:
                raise ValueError("successful StoryGraph result is incomplete")
            computed = self.compute_result_hash()
            if self.result_hash != computed:
                raise ValueError(
                    f"StoryGraph result hash mismatch: got {self.result_hash} want {computed}"
                )
            return self
        if self.candidate is not None or self.result_hash is not None or self.error is None:
            raise ValueError("failed or unknown StoryGraph result is incomplete")
        deterministic = {
            "skill_bundle_invalid",
            "invocation_policy_invalid",
            "candidate_schema_invalid",
            "evidence_invalid",
            "upstream_candidate_stale",
            "execution_budget_exceeded",
            "execution_deadline_exceeded",
            "tool_not_allowed",
        }
        retryable_unknown = {
            "skill_bundle_unavailable",
            "runtime_unavailable",
            "agent_execution_unknown",
        }
        valid = (
            self.status == "failed"
            and not self.error.retryable
            and self.error.code in deterministic
        ) or (
            self.status == "unknown"
            and self.error.retryable
            and self.error.code in retryable_unknown
        )
        if not valid:
            raise ValueError("StoryGraph result error semantics are invalid")
        return self

    def compute_result_hash(self) -> str:
        if self.candidate is None:
            raise ValueError("StoryGraph result has no candidate")
        return canonical_hash(self.candidate)

    def validate_for(self, invocation: StoryGraphStageInvocation) -> None:
        if (
            self.invocation_id != invocation.invocation_id
            or self.stage != invocation.payload.stage
            or self.shard_key != invocation.payload.shard_key
            or self.input_hash != invocation.input_hash
        ):
            raise ValueError("StoryGraph result identity does not match invocation")


class StoryGraphExecutionGrantClaims(BaseModel):
    model_config = ConfigDict(extra="forbid")

    invocation_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    execution_policy_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    expires_at: int = Field(ge=1)
    attempt: int = Field(ge=1)
    fencing_token: int = Field(ge=1)

    def validate_for(self, invocation: StoryGraphStageInvocation, *, now_unix: int) -> None:
        if (
            self.invocation_id != invocation.invocation_id
            or self.input_hash != invocation.input_hash
            or self.execution_policy_hash != invocation.execution_policy.canonical_hash()
            or self.expires_at <= now_unix
        ):
            raise ValueError("invalid StoryGraph execution grant claims")
