from __future__ import annotations

import hashlib
from datetime import datetime
from typing import Any, Literal, Self
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.candidate_runtime.canonical import canonical_hash

SceneAnalysisStageKey = Literal["propose_script_spans", "extract_scene_facts"]


class StrictSceneAnalysisModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class SceneAnalysisStageVariant(StrictSceneAnalysisModel):
    stage_key: SceneAnalysisStageKey
    profile_key: Literal["default"]
    lane_key: Literal["primary"]
    output_schema_version: Literal[
        "script-span-candidate",
        "scene-fact-candidate",
    ]

    @model_validator(mode="after")
    def validate_schema_for_stage(self) -> SceneAnalysisStageVariant:
        expected = {
            "propose_script_spans": "script-span-candidate",
            "extract_scene_facts": "scene-fact-candidate",
        }
        if self.output_schema_version != expected[self.stage_key]:
            raise ValueError("stage output schema does not match the Scene Analysis variant")
        return self


class ScriptSourceVersionIdentity(StrictSceneAnalysisModel):
    owner_kind: Literal["production/script-source"]
    logical_id: str = Field(min_length=1)
    version_id: UUID
    revision: int = Field(ge=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    created_at: datetime


class ScriptSpanRevisionIdentity(StrictSceneAnalysisModel):
    stage_key: Literal["propose_script_spans"]
    shard_key: str = Field(min_length=1)
    candidate_revision_id: UUID
    candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    source_invocation_id: UUID
    source_result_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class SceneAnalysisReleaseIdentity(StrictSceneAnalysisModel):
    release_id: UUID
    definition_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    bundle_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    agent_image_digest: str = Field(pattern=r"^sha256:[0-9a-f]{64}$")


class SceneAnalysisControlProof(StrictSceneAnalysisModel):
    record_id: UUID
    revision: int = Field(ge=1)
    status: Literal["approved"]
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class SceneAnalysisExecutionBudget(StrictSceneAnalysisModel):
    max_model_calls: int = Field(ge=1, le=2)
    max_execution_seconds: int = Field(ge=1, le=600)
    max_output_bytes: int = Field(ge=1024, le=1_048_576)


class SceneAnalysisScope(StrictSceneAnalysisModel):
    workspace_id: UUID
    project_id: UUID
    episode_id: UUID | None


class SceneAnalysisShard(StrictSceneAnalysisModel):
    manifest_id: UUID
    manifest_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    shard_key: str = Field(min_length=1)
    codepoint_start: int = Field(ge=0)
    codepoint_end: int = Field(gt=0)

    @model_validator(mode="after")
    def validate_range(self) -> SceneAnalysisShard:
        if self.codepoint_end <= self.codepoint_start:
            raise ValueError("Scene Analysis shard range must be increasing")
        return self


class ScriptSpanProposalInput(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    normalized_text: str = Field(min_length=1)
    codepoint_count: int = Field(gt=0)
    newline_normalization: Literal["lf"]

    @model_validator(mode="after")
    def validate_source(self) -> ScriptSpanProposalInput:
        if len(self.normalized_text) != self.codepoint_count:
            raise ValueError("source codepoint count does not match normalized text")
        if hashlib.sha256(self.normalized_text.encode("utf-8")).hexdigest() != self.source_hash:
            raise ValueError("source hash does not match normalized text")
        if "\r" in self.normalized_text:
            raise ValueError("normalized Scene Analysis source must use LF line endings")
        return self


class SceneFactExtractionInput(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    normalized_text: str = Field(min_length=1)
    span_candidate_revision_id: UUID
    span_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    span_candidate: dict[str, Any]


class SceneAnalysisPayload(StrictSceneAnalysisModel):
    variant: SceneAnalysisStageVariant
    scope: SceneAnalysisScope
    source_refs: list[ScriptSourceVersionIdentity] = Field(min_length=1, max_length=1)
    upstream_candidates: list[ScriptSpanRevisionIdentity]
    shard: SceneAnalysisShard
    stage_input: dict[str, Any]

    @model_validator(mode="after")
    def validate_stage_input(self) -> SceneAnalysisPayload:
        source = self.source_refs[0]
        if self.variant.stage_key == "propose_script_spans":
            value = ScriptSpanProposalInput.model_validate(self.stage_input)
            if (
                self.upstream_candidates
                or source.version_id != value.source_version_id
                or source.content_hash != value.source_hash
                or self.shard.codepoint_start != 0
                or self.shard.codepoint_end != value.codepoint_count
            ):
                raise ValueError("script span input does not match its frozen source")
        else:
            value = SceneFactExtractionInput.model_validate(self.stage_input)
            if (
                len(self.upstream_candidates) != 1
                or source.version_id != value.source_version_id
                or source.content_hash != value.source_hash
                or self.upstream_candidates[0].candidate_revision_id
                != value.span_candidate_revision_id
                or self.upstream_candidates[0].candidate_revision_hash
                != value.span_candidate_revision_hash
                or self.shard.codepoint_start != 0
                or self.shard.codepoint_end != len(value.normalized_text)
            ):
                raise ValueError("scene fact input does not match its frozen spans")
        return self


class SceneAnalysisInvocation(StrictSceneAnalysisModel):
    invocation_id: UUID
    attempt_id: UUID
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-scene-analysis-wire"]
    stage_release: SceneAnalysisReleaseIdentity
    control: SceneAnalysisControlProof
    budget: SceneAnalysisExecutionBudget
    payload: SceneAnalysisPayload
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @classmethod
    def build(
        cls,
        *,
        invocation_id: UUID,
        attempt_id: UUID,
        stage_release: SceneAnalysisReleaseIdentity,
        control: SceneAnalysisControlProof,
        budget: SceneAnalysisExecutionBudget,
        payload: SceneAnalysisPayload,
    ) -> Self:
        material = {
            "wire_schema_version": "storygraph-scene-analysis-wire",
            "stage_release": stage_release.model_dump(mode="json"),
            "control": control.model_dump(mode="json"),
            "budget": budget.model_dump(mode="json"),
            "payload": _canonical_payload(payload),
        }
        return cls(
            invocation_id=invocation_id,
            attempt_id=attempt_id,
            kind="storygraph_stage",
            wire_schema_version="storygraph-scene-analysis-wire",
            stage_release=stage_release,
            control=control,
            budget=budget,
            payload=payload,
            input_hash=canonical_hash(material),
        )

    @model_validator(mode="after")
    def validate_input_hash(self) -> SceneAnalysisInvocation:
        if self.input_hash != self.compute_input_hash():
            raise ValueError("scene analysis input hash mismatch")
        return self

    def compute_input_hash(self) -> str:
        return canonical_hash(
            {
                "wire_schema_version": self.wire_schema_version,
                "stage_release": self.stage_release.model_dump(mode="json"),
                "control": self.control.model_dump(mode="json"),
                "budget": self.budget.model_dump(mode="json"),
                "payload": _canonical_payload(self.payload),
            }
        )

    def stage_instance_key(self) -> str:
        material = (
            "storygraph-scene-analysis-stage"
            + self.payload.variant.stage_key
            + self.payload.variant.profile_key
            + self.payload.variant.lane_key
            + self.payload.variant.output_schema_version
            + self.payload.shard.shard_key
            + self.payload.shard.manifest_hash
            + self.input_hash
        )
        return hashlib.sha256(material.encode("utf-8")).hexdigest()


class SceneAnalysisExecutionGrantClaims(StrictSceneAnalysisModel):
    invocation_id: UUID
    attempt_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    stage_release_id: UUID
    agent_image_digest: str = Field(pattern=r"^sha256:[0-9a-f]{64}$")
    expires_at: int = Field(ge=1)

    def validate_for(self, invocation: SceneAnalysisInvocation, *, now_unix: int) -> None:
        if (
            self.invocation_id != invocation.invocation_id
            or self.attempt_id != invocation.attempt_id
            or self.input_hash != invocation.input_hash
            or self.stage_release_id != invocation.stage_release.release_id
            or self.agent_image_digest != invocation.stage_release.agent_image_digest
            or self.expires_at <= now_unix
        ):
            raise ValueError("invalid Scene Analysis execution grant claims")


class SceneAnalysisDiagnostic(StrictSceneAnalysisModel):
    code: str = Field(pattern=r"^[a-z][a-z0-9_]{1,80}$")
    summary: str = Field(min_length=1, max_length=800)


class SceneAnalysisResultError(StrictSceneAnalysisModel):
    code: str = Field(pattern=r"^[a-z][a-z0-9_]{1,80}$")
    safe_summary: str = Field(min_length=1, max_length=800)
    retry_class: Literal["never", "same_release"]


class SceneAnalysisExecutor(StrictSceneAnalysisModel):
    runtime_class: Literal["text"]
    runtime_image_digest: str = Field(pattern=r"^sha256:[0-9a-f]{64}$")
    harness_version: Literal["scene-analysis-harness"]
    model: str = Field(min_length=1)


class SceneAnalysisAttemptResult(StrictSceneAnalysisModel):
    invocation_id: UUID
    attempt_id: UUID
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-scene-analysis-wire"]
    variant: SceneAnalysisStageVariant
    stage_release: SceneAnalysisReleaseIdentity
    control: SceneAnalysisControlProof
    status: Literal["accepted", "rejected", "outcome_unknown"]
    candidate_type: Literal["script_span_candidate", "scene_fact_candidate"]
    candidate: dict[str, Any] | None
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    output_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    diagnostics: list[SceneAnalysisDiagnostic]
    diagnostic_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    completed_at: datetime
    executor: SceneAnalysisExecutor
    error: SceneAnalysisResultError | None

    @model_validator(mode="after")
    def validate_result_state(self) -> SceneAnalysisAttemptResult:
        expected_diagnostic_hash = canonical_hash(
            [value.model_dump(mode="json") for value in self.diagnostics]
        )
        if self.diagnostic_hash != expected_diagnostic_hash:
            raise ValueError("Scene Analysis diagnostic hash mismatch")
        if self.status == "accepted":
            if (
                self.candidate is None
                or self.output_hash is None
                or self.error is not None
                or canonical_hash(self.candidate) != self.output_hash
            ):
                raise ValueError("accepted Scene Analysis result is incomplete")
        elif (
            self.candidate is not None
            or self.output_hash is not None
            or self.error is None
            or (self.status == "rejected" and self.error.retry_class != "never")
            or (self.status == "outcome_unknown" and self.error.retry_class != "same_release")
        ):
            raise ValueError("failed Scene Analysis result has invalid semantics")
        return self

    def validate_for(self, invocation: SceneAnalysisInvocation) -> None:
        from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec

        if (
            self.invocation_id != invocation.invocation_id
            or self.attempt_id != invocation.attempt_id
            or self.variant != invocation.payload.variant
            or self.stage_release != invocation.stage_release
            or self.control != invocation.control
            or self.input_hash != invocation.input_hash
            or self.candidate_type
            != scene_analysis_stage_spec(invocation.payload.variant.stage_key).candidate_type
            or self.executor.runtime_image_digest != invocation.stage_release.agent_image_digest
        ):
            raise ValueError("Scene Analysis result identity does not match invocation")


def _canonical_payload(payload: SceneAnalysisPayload) -> dict[str, Any]:
    value = payload.model_dump(mode="json")
    value["source_refs"] = sorted(
        value["source_refs"],
        key=lambda item: (
            item["owner_kind"],
            item["logical_id"],
            item["version_id"],
            item["revision"],
            item["content_hash"],
        ),
    )
    value["upstream_candidates"] = sorted(
        value["upstream_candidates"],
        key=lambda item: (
            item["stage_key"],
            item["shard_key"],
            item["candidate_revision_id"],
            item["candidate_revision_hash"],
        ),
    )
    return value
