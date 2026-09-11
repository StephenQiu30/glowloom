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
from app.modules.storygraph.visual_foundation_contract import (
    VisualFoundationCandidate,
    VisualFoundationInput,
)
from app.protocol.canonical import production_canonical_hash

MAX_VISUAL_FOUNDATION_IMAGES = 8
MAX_VISUAL_FOUNDATION_IMAGE_BYTES = 10 * 1024 * 1024
MAX_VISUAL_FOUNDATION_TOTAL_IMAGE_BYTES = 32 * 1024 * 1024


class VisualFoundationStageVariant(StrictSceneAnalysisModel):
    stage_key: Literal["resolve_visual_foundation"]
    profile_key: Literal["default"]
    lane_key: Literal["primary"]
    output_schema_version: Literal["visual-foundation-candidate-production"]


class VisualFoundationScope(StrictSceneAnalysisModel):
    workspace_id: UUID
    project_id: UUID


class VisualFoundationShard(StrictSceneAnalysisModel):
    manifest_id: UUID
    manifest_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    shard_key: str = Field(pattern=r"^project:[0-9a-f-]{36}$")
    impact_closure_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class VisualFoundationMediaAttachment(StrictSceneAnalysisModel):
    attachment_id: UUID
    media_object_id: UUID
    version_no: int = Field(ge=1)
    purpose: Literal["style_reference"]
    object_key: str = Field(min_length=1, max_length=512)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    media_type: Literal["image/jpeg", "image/png", "image/webp"]
    byte_length: int = Field(ge=1, le=MAX_VISUAL_FOUNDATION_IMAGE_BYTES)
    pixel_width: int = Field(ge=1, le=16384)
    pixel_height: int = Field(ge=1, le=16384)
    page_count: Literal[1]
    frame_count: Literal[1]
    rights_basis: Literal["licensed", "owned", "public_domain"]
    rights_ref_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    lineage_ref_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class VisualFoundationPayload(StrictSceneAnalysisModel):
    variant: VisualFoundationStageVariant
    scope: VisualFoundationScope
    shard: VisualFoundationShard
    media_attachments: list[VisualFoundationMediaAttachment] = Field(
        max_length=MAX_VISUAL_FOUNDATION_IMAGES
    )
    stage_input: VisualFoundationInput

    @model_validator(mode="after")
    def validate_frozen_media(self) -> VisualFoundationPayload:
        if (
            self.scope.workspace_id != self.stage_input.workspace_id
            or self.scope.project_id != self.stage_input.project_id
            or self.shard.shard_key != f"project:{self.scope.project_id}"
            or sum(value.byte_length for value in self.media_attachments)
            > MAX_VISUAL_FOUNDATION_TOTAL_IMAGE_BYTES
        ):
            raise ValueError("Visual Foundation Project scope or media budget drifted")
        if len({value.attachment_id for value in self.media_attachments}) != len(
            self.media_attachments
        ) or len({value.media_object_id for value in self.media_attachments}) != len(
            self.media_attachments
        ):
            raise ValueError("Visual Foundation media identities must be unique")
        if len(self.media_attachments) != len(self.stage_input.reference_attachments):
            raise ValueError("Visual Foundation media manifest is incomplete")
        for manifest, reference in zip(
            self.media_attachments,
            self.stage_input.reference_attachments,
            strict=True,
        ):
            if (
                manifest.attachment_id != reference.attachment_id
                or manifest.object_key != reference.object_key
                or manifest.content_hash != reference.content_hash
                or manifest.media_type != reference.media_type
                or manifest.rights_basis != reference.rights_basis
                or manifest.rights_ref_hash != reference.rights_ref_hash
            ):
                raise ValueError("Visual Foundation media manifest drifted from stage input")
        return self


class VisualFoundationInvocation(StrictSceneAnalysisModel):
    invocation_id: UUID
    attempt_id: UUID
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-stage-wire-production"]
    stage_release: SceneAnalysisReleaseIdentity
    control: SceneAnalysisControlProof
    budget: SceneAnalysisExecutionBudget
    payload: VisualFoundationPayload
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
        payload: VisualFoundationPayload,
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
    def validate_execution_identity(self) -> VisualFoundationInvocation:
        if (
            self.stage_release.bundle_content_hash != SKILL_BUNDLE_HASH
            or self.budget.max_model_calls != 1
            or self.budget.max_execution_seconds > 120
            or self.budget.max_output_bytes > 131072
            or self.input_hash != self.compute_input_hash()
        ):
            raise ValueError("Visual Foundation invocation policy or input hash drifted")
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


def validate_visual_foundation_dispatch_authorization(
    claims: SceneAnalysisDispatchAuthorizationClaims,
    invocation: VisualFoundationInvocation,
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
        raise ValueError("invalid Visual Foundation dispatch authorization claims")


class VisualFoundationExecutor(StrictSceneAnalysisModel):
    runtime_class: Literal["vision"]
    runtime_image_digest: str = Field(pattern=r"^sha256:[0-9a-f]{64}$")
    harness_version: Literal["visual-foundation-harness"]
    model: str = Field(min_length=1, max_length=200)


class VisualFoundationAttemptResult(StrictSceneAnalysisModel):
    invocation_id: UUID
    attempt_id: UUID
    kind: Literal["storygraph_stage"]
    wire_schema_version: Literal["storygraph-stage-wire-production"]
    variant: VisualFoundationStageVariant
    stage_release: SceneAnalysisReleaseIdentity
    control: SceneAnalysisControlProof
    claim_version: int = Field(ge=1)
    dispatch_authorization_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    status: Literal["accepted", "rejected", "outcome_unknown"]
    candidate_type: Literal["visual_foundation_candidate"]
    candidate: dict[str, Any] | None
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    output_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    diagnostics: list[SceneAnalysisDiagnostic]
    diagnostic_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    completed_at: datetime
    executor: VisualFoundationExecutor
    error: SceneAnalysisResultError | None
    result_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    @classmethod
    def build(cls, **values: Any) -> Self:
        draft = cls.model_construct(result_hash="0" * 64, **values)
        material = draft.model_dump(mode="json", exclude={"result_hash"})
        return cls.model_validate({**material, "result_hash": production_canonical_hash(material)})

    @model_validator(mode="after")
    def validate_result_state(self) -> VisualFoundationAttemptResult:
        self._validate_result_state()
        return self

    def _validate_result_state(self) -> None:
        if self.result_hash != self.compute_result_hash():
            raise ValueError("Visual Foundation result hash mismatch")
        if self.diagnostic_hash != production_canonical_hash(
            [value.model_dump(mode="json") for value in self.diagnostics]
        ):
            raise ValueError("Visual Foundation diagnostic hash mismatch")
        if self.status == "accepted":
            if (
                self.candidate is None
                or self.output_hash is None
                or self.error is not None
                or production_canonical_hash(self.candidate) != self.output_hash
            ):
                raise ValueError("accepted Visual Foundation result is incomplete")
            VisualFoundationCandidate.model_validate(self.candidate)
        elif (
            self.candidate is not None
            or self.output_hash is not None
            or self.error is None
            or (self.status == "rejected" and self.error.retry_class != "never")
            or (self.status == "outcome_unknown" and self.error.retry_class != "same_release")
        ):
            raise ValueError("failed Visual Foundation result has invalid semantics")

    def compute_result_hash(self) -> str:
        return production_canonical_hash(self.model_dump(mode="json", exclude={"result_hash"}))

    def validate_for(
        self,
        invocation: VisualFoundationInvocation,
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
            raise ValueError("Visual Foundation result identity does not match invocation")
        if self.candidate is not None:
            candidate = VisualFoundationCandidate.model_validate(self.candidate)
            candidate.validate_for(invocation.payload.stage_input)
