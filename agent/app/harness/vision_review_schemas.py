from __future__ import annotations

from datetime import datetime
from typing import Annotated, Any, Literal, Self
from uuid import UUID

from pydantic import BeforeValidator, Field, model_validator

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
from app.modules.storygraph.vision_review_contract import VisionReviewCandidate
from app.modules.storygraph.vision_review_input import VisionReviewInput
from app.protocol.canonical import production_canonical_hash, production_canonical_json_text


def _canonical_identity(value: Any) -> UUID:
    if not isinstance(value, (str, UUID)):
        raise ValueError("Vision Review identity must be a canonical UUID")
    identifier = UUID(str(value))
    if identifier.int == 0 or str(identifier) != str(value):
        raise ValueError("Vision Review identity must be a nonempty canonical UUID")
    return identifier


VisionReviewIdentity = Annotated[UUID, BeforeValidator(_canonical_identity)]


class VisionReviewStageVariant(StrictSceneAnalysisModel):
    stage_key: Literal["review_reference_artifact"]
    profile_key: Literal["default"]
    lane_key: Literal["primary"]
    output_schema_version: Literal["vision-review-candidate-production"]


class VisionReviewScope(StrictSceneAnalysisModel):
    workspace_id: VisionReviewIdentity
    project_id: VisionReviewIdentity
    bundle_input_id: VisionReviewIdentity


class VisionReviewShard(StrictSceneAnalysisModel):
    manifest_id: VisionReviewIdentity
    manifest_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    shard_key: str = Field(min_length=1)
    impact_closure_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class VisionReviewPayload(StrictSceneAnalysisModel):
    variant: VisionReviewStageVariant
    scope: VisionReviewScope
    shard: VisionReviewShard
    stage_input: VisionReviewInput

    @model_validator(mode="after")
    def validate_frozen_input(self) -> VisionReviewPayload:
        if (
            str(self.scope.workspace_id) != self.stage_input.subject.workspace_id
            or str(self.scope.project_id) != self.stage_input.subject.project_id
            or str(self.scope.bundle_input_id) != self.stage_input.subject.bundle_input_ref.id
            or self.shard.shard_key != f"vision_bundle:{self.scope.bundle_input_id}"
        ):
            raise ValueError("Vision Review Target scope drifted")
        return self


class VisionReviewInvocation(StrictSceneAnalysisModel):
    invocation_id: VisionReviewIdentity
    attempt_id: VisionReviewIdentity
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-stage-wire-production"]
    stage_release: SceneAnalysisReleaseIdentity
    control: SceneAnalysisControlProof
    budget: SceneAnalysisExecutionBudget
    payload: VisionReviewPayload
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
        payload: VisionReviewPayload,
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
    def validate_execution_identity(self) -> VisionReviewInvocation:
        if (
            self.stage_release.bundle_content_hash != SKILL_BUNDLE_HASH
            or self.payload.stage_input.subject.stage_release_hash
            != self.stage_release.stage_release_hash
            or self.budget.max_model_calls != 1
            or self.budget.max_execution_seconds > 120
            or self.budget.max_output_bytes > 131072
            or self.input_hash != self.compute_input_hash()
        ):
            raise ValueError("Vision Review invocation policy or input hash drifted")
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


def validate_vision_review_dispatch_authorization(
    claims: SceneAnalysisDispatchAuthorizationClaims,
    invocation: VisionReviewInvocation,
    *,
    claim_version: int,
    now_unix: int,
) -> None:
    VisionReviewInvocation.model_validate(invocation.model_dump(mode="json"))
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
        raise ValueError("invalid Vision Review dispatch authorization claims")


def decode_vision_review_invocation(raw: str | bytes) -> VisionReviewInvocation:
    canonical = production_canonical_json_text(raw.encode() if isinstance(raw, str) else raw)
    value = VisionReviewInvocation.model_validate_json(canonical)
    if canonical != production_canonical_json_text(value.model_dump_json().encode()):
        raise ValueError("Vision Review invocation has missing or noncanonical fields")
    return value


class VisionReviewExecutor(StrictSceneAnalysisModel):
    runtime_class: Literal["vision"]
    runtime_image_digest: str = Field(pattern=r"^sha256:[0-9a-f]{64}$")
    harness_version: Literal["vision-review-harness"]
    model: str = Field(min_length=1, max_length=200)

    @model_validator(mode="after")
    def validate_model(self) -> VisionReviewExecutor:
        if not self.model.strip() or len(self.model.encode("utf-8")) > 200:
            raise ValueError("Vision Review executor model is invalid")
        return self


class VisionReviewAttemptResult(StrictSceneAnalysisModel):
    invocation_id: VisionReviewIdentity
    attempt_id: VisionReviewIdentity
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-stage-wire-production"]
    variant: VisionReviewStageVariant
    stage_release: SceneAnalysisReleaseIdentity
    control: SceneAnalysisControlProof
    claim_version: int = Field(ge=1)
    dispatch_authorization_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    status: Literal["accepted", "rejected", "outcome_unknown"]
    candidate_type: Literal["vision_review_candidate"]
    candidate: dict[str, Any] | None
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    output_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    diagnostics: list[SceneAnalysisDiagnostic]
    diagnostic_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    completed_at: datetime
    executor: VisionReviewExecutor
    error: SceneAnalysisResultError | None
    result_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @classmethod
    def build(cls, **values: Any) -> Self:
        draft = cls.model_construct(result_hash="0" * 64, **values)
        material = draft.model_dump(mode="json", exclude={"result_hash"})
        return cls.model_validate({**material, "result_hash": production_canonical_hash(material)})

    @model_validator(mode="after")
    def validate_result_state(self) -> VisionReviewAttemptResult:
        self._validate_result_state()
        return self

    def _validate_result_state(self) -> None:
        if (
            self.executor.runtime_image_digest != self.stage_release.agent_image_digest
            or self.completed_at.utcoffset() is None
            or self.completed_at == datetime.min.replace(tzinfo=self.completed_at.tzinfo)
        ):
            raise ValueError("Vision Review executor or completion identity is invalid")
        if self.result_hash != self.compute_result_hash():
            raise ValueError("Vision Review result hash mismatch")
        if self.diagnostic_hash != production_canonical_hash(
            [value.model_dump(mode="json") for value in self.diagnostics]
        ):
            raise ValueError("Vision Review diagnostic hash mismatch")
        if self.status == "accepted":
            if (
                self.candidate is None
                or self.output_hash is None
                or self.error is not None
                or production_canonical_hash(self.candidate) != self.output_hash
            ):
                raise ValueError("accepted Vision Review result is incomplete")
            VisionReviewCandidate.model_validate(self.candidate)
        elif (
            self.candidate is not None
            or self.output_hash is not None
            or self.error is None
            or (self.status == "rejected" and self.error.retry_class != "never")
            or (self.status == "outcome_unknown" and self.error.retry_class != "same_release")
        ):
            raise ValueError("failed Vision Review result has invalid semantics")

    def compute_result_hash(self) -> str:
        return production_canonical_hash(self.model_dump(mode="json", exclude={"result_hash"}))

    def validate_for(
        self,
        invocation: VisionReviewInvocation,
        claim_version: int,
        dispatch_authorization_hash: str,
    ) -> None:
        VisionReviewInvocation.model_validate(invocation.model_dump(mode="json"))
        VisionReviewAttemptResult.model_validate(self.model_dump(mode="json"))
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
            raise ValueError("Vision Review result identity does not match invocation")
        if self.candidate is not None:
            candidate = VisionReviewCandidate.model_validate(self.candidate)
            candidate.validate_for(invocation.payload.stage_input.subject)


def decode_vision_review_attempt_result(raw: str | bytes) -> VisionReviewAttemptResult:
    canonical = production_canonical_json_text(raw.encode() if isinstance(raw, str) else raw)
    value = VisionReviewAttemptResult.model_validate_json(canonical)
    if canonical != production_canonical_json_text(value.model_dump_json().encode()):
        raise ValueError("Vision Review result has missing or noncanonical fields")
    return value
