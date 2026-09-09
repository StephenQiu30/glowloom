from __future__ import annotations

import hashlib
import re
from datetime import datetime
from typing import Any, Literal, Self
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.modules.storygraph.scene_analysis_candidates import CandidateReviewIssue
from app.protocol.canonical import production_canonical_hash

SceneAnalysisStageKey = Literal[
    "propose_script_spans",
    "extract_scene_facts",
    "resolve_identities",
    "review_candidate",
]


class StrictSceneAnalysisModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class SceneAnalysisStageVariant(StrictSceneAnalysisModel):
    stage_key: SceneAnalysisStageKey
    profile_key: Literal["default", "structure_identity"]
    lane_key: Literal["primary"]
    output_schema_version: Literal[
        "script-span-candidate-production",
        "scene-fact-candidate-production",
        "identity-resolution-candidate-production",
        "structure-identity-review-candidate-production",
    ]

    @model_validator(mode="after")
    def validate_output_schema(self) -> SceneAnalysisStageVariant:
        expected = {
            ("propose_script_spans", "default"): "script-span-candidate-production",
            ("extract_scene_facts", "default"): "scene-fact-candidate-production",
            ("resolve_identities", "default"): "identity-resolution-candidate-production",
            (
                "review_candidate",
                "structure_identity",
            ): "structure-identity-review-candidate-production",
        }.get((self.stage_key, self.profile_key))
        if self.output_schema_version != expected:
            raise ValueError("Scene Analysis output schema does not match its stage")
        return self


class ScriptSourceVersionIdentity(StrictSceneAnalysisModel):
    owner_kind: Literal["production/script"]
    logical_id: str = Field(min_length=1)
    version_id: UUID
    revision: int = Field(ge=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    created_at: datetime


class SceneAnalysisCandidateRevisionIdentity(StrictSceneAnalysisModel):
    stage_key: Literal[
        "propose_script_spans", "extract_scene_facts", "resolve_identities", "review_candidate"
    ]
    shard_key: str = Field(min_length=1)
    candidate_revision_id: UUID
    candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    source_invocation_id: UUID
    source_result_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class SceneAnalysisReleaseIdentity(StrictSceneAnalysisModel):
    skill_release_id: UUID
    skill_release_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    stage_release_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    bundle_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    agent_image_digest: str = Field(pattern=r"^sha256:[0-9a-f]{64}$")


class SceneAnalysisControlProof(StrictSceneAnalysisModel):
    control_record_id: UUID
    control_revision: int = Field(ge=1)
    status: Literal["approved"]
    control_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    release_fence: int = Field(ge=0)


class SceneAnalysisExecutionBudget(StrictSceneAnalysisModel):
    max_attempts: int = Field(ge=1, le=3)
    max_model_calls: int = Field(ge=1, le=2)
    max_execution_seconds: int = Field(ge=1, le=600)
    max_output_bytes: int = Field(ge=1024, le=1_048_576)


class SceneAnalysisScope(StrictSceneAnalysisModel):
    workspace_id: UUID
    project_id: UUID
    episode_id: UUID | None
    scene_id: UUID | None
    entity_id: UUID | None
    target_id: UUID | None

    @model_validator(mode="after")
    def validate_hierarchy(self) -> SceneAnalysisScope:
        if self.episode_id is None and any(
            value is not None for value in (self.scene_id, self.entity_id, self.target_id)
        ):
            raise ValueError("nested Scene Analysis scope requires an episode")
        return self


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


class StructureIdentityRepairEvidence(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    text_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @model_validator(mode="after")
    def validate_range(self) -> StructureIdentityRepairEvidence:
        if self.source_end <= self.source_start:
            raise ValueError("repair evidence range must be increasing")
        return self


class StructureIdentityRepairChange(StrictSceneAnalysisModel):
    operation: Literal[
        "inspect_source",
        "adjust_episode_boundary",
        "adjust_scene_boundary",
        "separate_identity",
        "merge_identity",
        "resolve_mention",
        "reject_mention",
    ]
    target_keys: list[str]
    affected_scope_keys: list[str]

    @model_validator(mode="after")
    def validate_targets(self) -> StructureIdentityRepairChange:
        if (
            not self.target_keys
            or self.target_keys != sorted(set(self.target_keys))
            or any(not value.strip() for value in self.target_keys)
        ):
            raise ValueError("repair target keys must be non-empty, sorted, and unique")
        if not self.affected_scope_keys or self.affected_scope_keys != sorted(
            set(self.affected_scope_keys)
        ):
            raise ValueError("repair scopes must be non-empty, sorted, and unique")
        for scope in self.affected_scope_keys:
            prefix, separator, identifier = scope.partition(":")
            if prefix != "scene" or separator != ":":
                raise ValueError("repair scope must identify one frozen scene")
            UUID(identifier)
        return self


class StructureIdentityRepairDirective(StrictSceneAnalysisModel):
    review_decision_id: UUID
    decision_payload_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    issue_refs: list[str]
    evidence_refs: list[StructureIdentityRepairEvidence]
    change_spec: StructureIdentityRepairChange
    reason_code: Literal[
        "source_interpretation_incorrect",
        "insufficient_evidence",
        "structure_boundary_incorrect",
        "identity_resolution_incorrect",
    ]

    @model_validator(mode="after")
    def validate_directive(self) -> StructureIdentityRepairDirective:
        if (
            not self.issue_refs
            or self.issue_refs != sorted(set(self.issue_refs))
            or any(not re.fullmatch(r"issue_[a-z0-9_]{1,80}", value) for value in self.issue_refs)
        ):
            raise ValueError("repair issue refs must be semantic, sorted, and unique")
        evidence_order = [
            (str(value.source_version_id), value.source_start, value.source_end, value.text_hash)
            for value in self.evidence_refs
        ]
        if not evidence_order or evidence_order != sorted(set(evidence_order)):
            raise ValueError("repair evidence refs must be non-empty, sorted, and unique")
        span_repair = self.change_spec.operation in {
            "inspect_source",
            "adjust_episode_boundary",
            "adjust_scene_boundary",
        }
        if span_repair and self.reason_code not in {
            "source_interpretation_incorrect",
            "insufficient_evidence",
            "structure_boundary_incorrect",
        }:
            raise ValueError("span repair reason does not match its operation")
        if not span_repair and self.reason_code != "identity_resolution_incorrect":
            raise ValueError("identity repair reason does not match its operation")
        return self

    def validate_for(self, stage_key: SceneAnalysisStageKey) -> None:
        span_repair = self.change_spec.operation in {
            "inspect_source",
            "adjust_episode_boundary",
            "adjust_scene_boundary",
        }
        identity_repair = not span_repair
        if (stage_key == "propose_script_spans") != span_repair or (
            stage_key == "resolve_identities"
        ) != identity_repair:
            raise ValueError("repair operation targets another Scene Analysis stage")


class ScriptSpanProposalInput(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    normalized_text: str = Field(min_length=1)
    codepoint_count: int = Field(gt=0)
    newline_normalization: Literal["lf"]
    repair: StructureIdentityRepairDirective | None = None

    @model_validator(mode="after")
    def validate_source(self) -> ScriptSpanProposalInput:
        if len(self.normalized_text) != self.codepoint_count:
            raise ValueError("source codepoint count does not match normalized text")
        if hashlib.sha256(self.normalized_text.encode("utf-8")).hexdigest() != self.source_hash:
            raise ValueError("source hash does not match normalized text")
        if "\r" in self.normalized_text:
            raise ValueError("normalized Scene Analysis source must use LF line endings")
        if self.repair is not None:
            self.repair.validate_for("propose_script_spans")
        return self


class SceneFactExtractionInput(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    normalized_text: str = Field(min_length=1)
    span_candidate_revision_id: UUID
    span_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    span_candidate: dict[str, Any]


class IdentityResolutionInput(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    normalized_text: str = Field(min_length=1)
    scene_fact_candidate_revision_id: UUID
    scene_fact_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_fact_candidate: dict[str, Any]
    allowed_reuse_identity_keys: list[str]
    repair: StructureIdentityRepairDirective | None = None

    @model_validator(mode="after")
    def validate_reuse_allowlist(self) -> IdentityResolutionInput:
        if self.allowed_reuse_identity_keys != sorted(set(self.allowed_reuse_identity_keys)) or any(
            not value.strip() for value in self.allowed_reuse_identity_keys
        ):
            raise ValueError("identity reuse allowlist must be sorted, unique, and non-empty")
        if self.repair is not None:
            self.repair.validate_for("resolve_identities")
        return self


class StructureIdentityReviewInput(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    normalized_text: str = Field(min_length=1)
    span_candidate_revision_id: UUID
    span_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    span_candidate: dict[str, Any]
    scene_fact_candidate_revision_id: UUID
    scene_fact_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_fact_candidate: dict[str, Any]
    identity_candidate_revision_id: UUID
    identity_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    identity_candidate: dict[str, Any]
    deterministic_issues: list[CandidateReviewIssue]

    @model_validator(mode="after")
    def validate_frozen_candidates(self) -> StructureIdentityReviewInput:
        from app.modules.storygraph.scene_analysis_candidates import (
            IdentityResolutionCandidate,
            SceneFactCandidate,
            ScriptSpanCandidate,
        )

        if hashlib.sha256(self.normalized_text.encode("utf-8")).hexdigest() != self.source_hash:
            raise ValueError("structure identity review source hash drifted")
        spans = ScriptSpanCandidate.model_validate(self.span_candidate)
        scene_facts = SceneFactCandidate.model_validate(self.scene_fact_candidate)
        identities = IdentityResolutionCandidate.model_validate(self.identity_candidate)
        issue_keys = [value.issue_key for value in self.deterministic_issues]
        if issue_keys != sorted(set(issue_keys)):
            raise ValueError("deterministic review issues must be unique and sorted")
        if (
            spans.source_version_id != self.source_version_id
            or spans.source_hash != self.source_hash
            or scene_facts.source_version_id != self.source_version_id
            or scene_facts.source_hash != self.source_hash
            or scene_facts.span_candidate_revision_id != self.span_candidate_revision_id
            or scene_facts.span_candidate_revision_hash != self.span_candidate_revision_hash
            or identities.source_version_id != self.source_version_id
            or identities.source_hash != self.source_hash
            or identities.scene_fact_candidate_revision_id != self.scene_fact_candidate_revision_id
            or identities.scene_fact_candidate_revision_hash
            != self.scene_fact_candidate_revision_hash
        ):
            raise ValueError("structure identity review candidate chain drifted")
        spans.validate_for_text(self.normalized_text)
        scene_facts.validate_for_spans(self.normalized_text, spans.spans)
        return self


class SceneAnalysisPayload(StrictSceneAnalysisModel):
    variant: SceneAnalysisStageVariant
    scope: SceneAnalysisScope
    source_refs: list[ScriptSourceVersionIdentity] = Field(min_length=1, max_length=1)
    upstream_candidates: list[SceneAnalysisCandidateRevisionIdentity]
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
        elif self.variant.stage_key == "extract_scene_facts":
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
        elif self.variant.stage_key == "resolve_identities":
            value = IdentityResolutionInput.model_validate(self.stage_input)
            if (
                len(self.upstream_candidates) != 1
                or self.upstream_candidates[0].stage_key != "extract_scene_facts"
                or source.version_id != value.source_version_id
                or source.content_hash != value.source_hash
                or self.upstream_candidates[0].candidate_revision_id
                != value.scene_fact_candidate_revision_id
                or self.upstream_candidates[0].candidate_revision_hash
                != value.scene_fact_candidate_revision_hash
                or self.shard.codepoint_start != 0
                or self.shard.codepoint_end != len(value.normalized_text)
            ):
                raise ValueError("identity input does not match its frozen SceneFacts")
        else:
            value = StructureIdentityReviewInput.model_validate(self.stage_input)
            expected = {
                "propose_script_spans": (
                    value.span_candidate_revision_id,
                    value.span_candidate_revision_hash,
                ),
                "extract_scene_facts": (
                    value.scene_fact_candidate_revision_id,
                    value.scene_fact_candidate_revision_hash,
                ),
                "resolve_identities": (
                    value.identity_candidate_revision_id,
                    value.identity_candidate_revision_hash,
                ),
            }
            supplied = {
                item.stage_key: (item.candidate_revision_id, item.candidate_revision_hash)
                for item in self.upstream_candidates
            }
            if (
                len(self.upstream_candidates) != 3
                or supplied != expected
                or source.version_id != value.source_version_id
                or source.content_hash != value.source_hash
                or self.shard.codepoint_start != 0
                or self.shard.codepoint_end != len(value.normalized_text)
            ):
                raise ValueError("review input does not match its frozen structure and identity")
        return self


class SceneAnalysisInvocation(StrictSceneAnalysisModel):
    invocation_id: UUID
    attempt_id: UUID
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-stage-wire-production"]
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
            "wire_schema_version": "storygraph-stage-wire-production",
            "stage_release": stage_release.model_dump(mode="json"),
            "control": control.model_dump(mode="json"),
            "budget": budget.model_dump(mode="json"),
            "payload": _canonical_payload(payload),
        }
        return cls(
            invocation_id=invocation_id,
            attempt_id=attempt_id,
            kind="storygraph_stage",
            wire_schema_version="storygraph-stage-wire-production",
            stage_release=stage_release,
            control=control,
            budget=budget,
            payload=payload,
            input_hash=production_canonical_hash(material),
        )

    @model_validator(mode="after")
    def validate_input_hash(self) -> SceneAnalysisInvocation:
        if self.input_hash != self.compute_input_hash():
            raise ValueError("scene analysis input hash mismatch")
        return self

    def compute_input_hash(self) -> str:
        return production_canonical_hash(
            {
                "wire_schema_version": self.wire_schema_version,
                "stage_release": self.stage_release.model_dump(mode="json"),
                "control": self.control.model_dump(mode="json"),
                "budget": self.budget.model_dump(mode="json"),
                "payload": _canonical_payload(self.payload),
            }
        )

    def stage_instance_key(self) -> str:
        return production_canonical_hash(
            {
                "identity_contract_id": "storygraph-stage-instance-production",
                "variant_key": self.payload.variant.model_dump(mode="json"),
                "scope": self.payload.scope.model_dump(mode="json"),
                "shard_manifest_hash": self.payload.shard.manifest_hash,
                "shard_key": self.payload.shard.shard_key,
                "input_hash": self.input_hash,
            }
        )


class SceneAnalysisDispatchAuthorizationClaims(StrictSceneAnalysisModel):
    invocation_id: UUID
    attempt_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    skill_release_id: UUID
    skill_release_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    stage_release_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    bundle_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    control_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    release_fence: int = Field(ge=0)
    claim_version: int = Field(ge=1)
    agent_image_digest: str = Field(pattern=r"^sha256:[0-9a-f]{64}$")
    expires_at: int = Field(ge=1)

    def validate_for(
        self,
        invocation: SceneAnalysisInvocation,
        *,
        now_unix: int,
    ) -> None:
        if (
            self.invocation_id != invocation.invocation_id
            or self.attempt_id != invocation.attempt_id
            or self.input_hash != invocation.input_hash
            or self.skill_release_id != invocation.stage_release.skill_release_id
            or self.skill_release_hash != invocation.stage_release.skill_release_hash
            or self.stage_release_hash != invocation.stage_release.stage_release_hash
            or self.bundle_content_hash != invocation.stage_release.bundle_content_hash
            or self.control_hash != invocation.control.control_hash
            or self.release_fence != invocation.control.release_fence
            or self.agent_image_digest != invocation.stage_release.agent_image_digest
            or self.expires_at <= now_unix
        ):
            raise ValueError("invalid Scene Analysis dispatch authorization claims")


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
    wire_schema_version: Literal["storygraph-stage-wire-production"]
    variant: SceneAnalysisStageVariant
    stage_release: SceneAnalysisReleaseIdentity
    control: SceneAnalysisControlProof
    claim_version: int = Field(ge=1)
    dispatch_authorization_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    status: Literal["accepted", "rejected", "outcome_unknown"]
    candidate_type: Literal[
        "script_span_candidate",
        "scene_fact_candidate",
        "identity_resolution_candidate",
        "structure_identity_review_candidate",
    ]
    candidate: dict[str, Any] | None
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    output_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    diagnostics: list[SceneAnalysisDiagnostic]
    diagnostic_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    completed_at: datetime
    executor: SceneAnalysisExecutor
    error: SceneAnalysisResultError | None
    result_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @classmethod
    def build(cls, **values: Any) -> Self:
        draft = cls.model_construct(result_hash="0" * 64, **values)
        material = draft.model_dump(mode="json", exclude={"result_hash"})
        return cls.model_validate({**material, "result_hash": production_canonical_hash(material)})

    def compute_result_hash(self) -> str:
        return production_canonical_hash(self.model_dump(mode="json", exclude={"result_hash"}))

    @model_validator(mode="after")
    def validate_result_state(self) -> SceneAnalysisAttemptResult:
        if self.result_hash != self.compute_result_hash():
            raise ValueError("Scene Analysis result hash mismatch")
        expected_diagnostic_hash = production_canonical_hash(
            [value.model_dump(mode="json") for value in self.diagnostics]
        )
        if self.diagnostic_hash != expected_diagnostic_hash:
            raise ValueError("Scene Analysis diagnostic hash mismatch")
        if self.status == "accepted":
            if (
                self.candidate is None
                or self.output_hash is None
                or self.error is not None
                or production_canonical_hash(self.candidate) != self.output_hash
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

    def validate_for(
        self,
        invocation: SceneAnalysisInvocation,
        claim_version: int,
        dispatch_authorization_hash: str,
    ) -> None:
        from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec

        if self.result_hash != self.compute_result_hash():
            raise ValueError("Scene Analysis result hash mismatch")
        if (
            self.invocation_id != invocation.invocation_id
            or self.attempt_id != invocation.attempt_id
            or self.variant != invocation.payload.variant
            or self.stage_release != invocation.stage_release
            or self.control != invocation.control
            or self.claim_version != claim_version
            or self.dispatch_authorization_hash != dispatch_authorization_hash
            or self.input_hash != invocation.input_hash
            or self.candidate_type
            != scene_analysis_stage_spec(
                invocation.payload.variant.stage_key,
                invocation.payload.variant.profile_key,
            ).candidate_type
            or self.executor.runtime_image_digest != invocation.stage_release.agent_image_digest
        ):
            raise ValueError("Scene Analysis result identity does not match invocation")


def _canonical_payload(payload: SceneAnalysisPayload) -> dict[str, Any]:
    value = payload.model_dump(mode="json")
    if value["stage_input"].get("repair") is None:
        value["stage_input"].pop("repair", None)
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
