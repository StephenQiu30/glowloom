from __future__ import annotations

from datetime import datetime
from typing import Any, Literal, Self
from uuid import UUID

from pydantic import Field, model_validator

from app.harness.scene_analysis_schemas import (
    SceneAnalysisControlProof,
    SceneAnalysisDiagnostic,
    SceneAnalysisDispatchAuthorizationClaims,
    SceneAnalysisExecutionBudget,
    SceneAnalysisReleaseIdentity,
    SceneAnalysisResultError,
    StrictSceneAnalysisModel,
)
from app.modules.storygraph.bundle import SKILL_BUNDLE_HASH
from app.modules.storygraph.reference_plan_contract import (
    ReferencePlanCandidate,
    ReferencePlanInput,
)
from app.protocol.canonical import production_canonical_hash


class ReferencePlanStageVariant(StrictSceneAnalysisModel):
    stage_key: Literal["plan_reference_assets"]
    profile_key: Literal["default"]
    lane_key: Literal["primary"]
    output_schema_version: Literal["reference-plan-candidate-production"]


class ReferencePlanScope(StrictSceneAnalysisModel):
    workspace_id: UUID
    project_id: UUID


class ReferencePlanShard(StrictSceneAnalysisModel):
    manifest_id: UUID
    manifest_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    shard_key: str = Field(pattern=r"^project:[0-9a-f-]{36}$")
    impact_closure_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class ReferencePlanPayload(StrictSceneAnalysisModel):
    variant: ReferencePlanStageVariant
    scope: ReferencePlanScope
    shard: ReferencePlanShard
    stage_input: ReferencePlanInput

    @model_validator(mode="after")
    def validate_frozen_input(self) -> ReferencePlanPayload:
        if (
            self.scope.workspace_id != self.stage_input.workspace_id
            or self.scope.project_id != self.stage_input.project_id
            or self.shard.shard_key != f"project:{self.scope.project_id}"
        ):
            raise ValueError("Reference Plan Project scope drifted")
        return self


class ReferencePlanInvocation(StrictSceneAnalysisModel):
    invocation_id: UUID
    attempt_id: UUID
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-stage-wire-production"]
    stage_release: SceneAnalysisReleaseIdentity
    control: SceneAnalysisControlProof
    budget: SceneAnalysisExecutionBudget
    payload: ReferencePlanPayload
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
        payload: ReferencePlanPayload,
    ) -> Self:
        draft = cls.model_construct(
            invocation_id=invocation_id,
            attempt_id=attempt_id,
            kind="storygraph_stage",
            wire_schema_version="storygraph-stage-wire-production",
            stage_release=stage_release,
            control=control,
            budget=budget,
            payload=payload,
            input_hash="0" * 64,
        )
        return cls.model_validate(
            draft.model_copy(update={"input_hash": draft.compute_input_hash()}).model_dump(
                mode="json"
            )
        )

    @model_validator(mode="after")
    def validate_execution_identity(self) -> ReferencePlanInvocation:
        if (
            self.stage_release.bundle_content_hash != SKILL_BUNDLE_HASH
            or self.budget.max_model_calls != 1
            or self.budget.max_execution_seconds > 120
            or self.budget.max_output_bytes > 131072
            or self.input_hash != self.compute_input_hash()
        ):
            raise ValueError("Reference Plan invocation policy or input hash drifted")
        return self

    def compute_input_hash(self) -> str:
        return production_canonical_hash(
            {
                "wire_schema_version": self.wire_schema_version,
                "stage_release": self.stage_release.model_dump(mode="json"),
                "control": self.control.model_dump(mode="json"),
                "budget": self.budget.model_dump(mode="json"),
                "payload": self.payload.model_dump(mode="json"),
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


def validate_reference_plan_dispatch_authorization(
    claims: SceneAnalysisDispatchAuthorizationClaims,
    invocation: ReferencePlanInvocation,
    *,
    claim_version: int,
    now_unix: int,
) -> None:
    if (
        claim_version < 1
        or claims.invocation_id != invocation.invocation_id
        or claims.attempt_id != invocation.attempt_id
        or claims.input_hash != invocation.input_hash
        or claims.skill_release_id != invocation.stage_release.skill_release_id
        or claims.skill_release_hash != invocation.stage_release.skill_release_hash
        or claims.stage_release_hash != invocation.stage_release.stage_release_hash
        or claims.bundle_content_hash != invocation.stage_release.bundle_content_hash
        or claims.control_hash != invocation.control.control_hash
        or claims.release_fence != invocation.control.release_fence
        or claims.claim_version != claim_version
        or claims.agent_image_digest != invocation.stage_release.agent_image_digest
        or claims.expires_at <= now_unix
    ):
        raise ValueError("invalid Reference Plan dispatch authorization claims")


class ReferencePlanExecutor(StrictSceneAnalysisModel):
    runtime_class: Literal["text"]
    runtime_image_digest: str = Field(pattern=r"^sha256:[0-9a-f]{64}$")
    harness_version: Literal["reference-plan-harness"]
    model: str = Field(min_length=1, max_length=200)


class ReferencePlanAttemptResult(StrictSceneAnalysisModel):
    invocation_id: UUID
    attempt_id: UUID
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-stage-wire-production"]
    variant: ReferencePlanStageVariant
    stage_release: SceneAnalysisReleaseIdentity
    control: SceneAnalysisControlProof
    claim_version: int = Field(ge=1)
    dispatch_authorization_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    status: Literal["accepted", "rejected", "outcome_unknown"]
    candidate_type: Literal["reference_plan_candidate"]
    candidate: dict[str, Any] | None
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    output_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    diagnostics: list[SceneAnalysisDiagnostic]
    diagnostic_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    completed_at: datetime
    executor: ReferencePlanExecutor
    error: SceneAnalysisResultError | None
    result_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @classmethod
    def build(cls, **values: Any) -> Self:
        draft = cls.model_construct(result_hash="0" * 64, **values)
        material = draft.model_dump(mode="json", exclude={"result_hash"})
        return cls.model_validate({**material, "result_hash": production_canonical_hash(material)})

    @model_validator(mode="after")
    def validate_result_state(self) -> ReferencePlanAttemptResult:
        self._validate_result_state()
        return self

    def _validate_result_state(self) -> None:
        if self.result_hash != self.compute_result_hash():
            raise ValueError("Reference Plan result hash mismatch")
        if self.diagnostic_hash != production_canonical_hash(
            [value.model_dump(mode="json") for value in self.diagnostics]
        ):
            raise ValueError("Reference Plan diagnostic hash mismatch")
        if self.status == "accepted":
            if (
                self.candidate is None
                or self.output_hash is None
                or self.error is not None
                or production_canonical_hash(self.candidate) != self.output_hash
            ):
                raise ValueError("accepted Reference Plan result is incomplete")
            ReferencePlanCandidate.model_validate(self.candidate)
        elif (
            self.candidate is not None
            or self.output_hash is not None
            or self.error is None
            or (self.status == "rejected" and self.error.retry_class != "never")
            or (self.status == "outcome_unknown" and self.error.retry_class != "same_release")
        ):
            raise ValueError("failed Reference Plan result has invalid semantics")

    def compute_result_hash(self) -> str:
        return production_canonical_hash(self.model_dump(mode="json", exclude={"result_hash"}))

    def validate_for(
        self,
        invocation: ReferencePlanInvocation,
        claim_version: int,
        dispatch_authorization_hash: str,
    ) -> None:
        self._validate_result_state()
        if (
            self.invocation_id != invocation.invocation_id
            or self.attempt_id != invocation.attempt_id
            or self.variant != invocation.payload.variant
            or self.stage_release != invocation.stage_release
            or self.control != invocation.control
            or self.claim_version != claim_version
            or self.dispatch_authorization_hash != dispatch_authorization_hash
            or self.input_hash != invocation.input_hash
            or self.executor.runtime_image_digest != invocation.stage_release.agent_image_digest
        ):
            raise ValueError("Reference Plan result identity does not match invocation")
        if self.candidate is not None:
            candidate = ReferencePlanCandidate.model_validate(self.candidate)
            candidate.validate_for(invocation.payload.stage_input)
