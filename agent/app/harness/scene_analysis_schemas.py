from __future__ import annotations

import hashlib
import re
from collections.abc import Callable
from datetime import datetime
from typing import Any, Literal, Self
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.modules.storygraph.scene_analysis_candidates import (
    CandidateReviewIssue,
    SourceEvidenceSpan,
)
from app.protocol.canonical import production_canonical_hash

SceneAnalysisStageKey = Literal[
    "propose_script_spans",
    "extract_scene_facts",
    "resolve_identities",
    "review_candidate",
    "derive_production_entities",
    "bind_scene_occurrences",
    "reconcile_interaction_continuity",
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
        "production-entity-fragment-candidate-production",
        "scene-binding-fragment-candidate-production",
        "continuity-fragment-candidate-production",
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
            (
                "derive_production_entities",
                "default",
            ): "production-entity-fragment-candidate-production",
            (
                "bind_scene_occurrences",
                "default",
            ): "scene-binding-fragment-candidate-production",
            (
                "reconcile_interaction_continuity",
                "default",
            ): "continuity-fragment-candidate-production",
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
        "propose_script_spans",
        "extract_scene_facts",
        "resolve_identities",
        "review_candidate",
        "derive_production_entities",
        "bind_scene_occurrences",
        "reconcile_interaction_continuity",
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


ProductionWorldRepairOperation = Literal[
    "revise_production_entity",
    "rebind_scene_occurrence",
    "revise_interaction",
    "revise_continuity",
]


class ProductionWorldRepairChange(StrictSceneAnalysisModel):
    operation: ProductionWorldRepairOperation
    target_keys: list[str]

    @model_validator(mode="after")
    def validate_targets(self) -> ProductionWorldRepairChange:
        if (
            not self.target_keys
            or self.target_keys != sorted(set(self.target_keys))
            or any(not value.strip() for value in self.target_keys)
        ):
            raise ValueError("Production World repair targets must be sorted and unique")
        return self


class ProductionWorldRepairClosure(StrictSceneAnalysisModel):
    scene_scope_keys: list[str]
    entity_keys: list[str]
    state_keys: list[str]
    occurrence_keys: list[str]
    interaction_keys: list[str]
    continuity_keys: list[str]
    ledger_keys: list[str]

    @model_validator(mode="after")
    def validate_keys(self) -> ProductionWorldRepairClosure:
        for values in (
            self.scene_scope_keys,
            self.entity_keys,
            self.state_keys,
            self.occurrence_keys,
            self.interaction_keys,
            self.continuity_keys,
            self.ledger_keys,
        ):
            if values != sorted(set(values)) or any(not value.strip() for value in values):
                raise ValueError("Production World repair closure must be sorted and unique")
        return self


class ProductionWorldRepairBaseCandidate(StrictSceneAnalysisModel):
    identity: SceneAnalysisCandidateRevisionIdentity
    candidate_type: Literal[
        "production_entity_fragment_candidate",
        "scene_binding_fragment_candidate",
        "continuity_fragment_candidate",
    ]
    candidate_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    candidate: dict[str, Any]

    @model_validator(mode="after")
    def validate_content(self) -> ProductionWorldRepairBaseCandidate:
        if production_canonical_hash(self.candidate) != self.candidate_content_hash:
            raise ValueError("Production World repair base Candidate content drifted")
        return self


class ProductionWorldRepairDirective(StrictSceneAnalysisModel):
    review_decision_id: UUID
    decision_payload_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    issue_refs: list[str]
    evidence_refs: list[StructureIdentityRepairEvidence] = Field(min_length=1)
    change_spec: ProductionWorldRepairChange
    closure: ProductionWorldRepairClosure
    reason_code: Literal[
        "production_entity_incorrect",
        "scene_occurrence_incorrect",
        "interaction_incorrect",
        "continuity_incorrect",
    ]
    base_candidate: ProductionWorldRepairBaseCandidate

    @model_validator(mode="after")
    def validate_directive(self) -> ProductionWorldRepairDirective:
        if self.issue_refs != sorted(set(self.issue_refs)) or any(
            not value.strip() for value in self.issue_refs
        ):
            raise ValueError("Production World repair issues must be sorted and unique")
        evidence_order = [
            (str(value.source_version_id), value.source_start, value.source_end, value.text_hash)
            for value in self.evidence_refs
        ]
        if evidence_order != sorted(set(evidence_order)):
            raise ValueError("Production World repair evidence must be sorted and unique")
        expected_reason = {
            "revise_production_entity": "production_entity_incorrect",
            "rebind_scene_occurrence": "scene_occurrence_incorrect",
            "revise_interaction": "interaction_incorrect",
            "revise_continuity": "continuity_incorrect",
        }[self.change_spec.operation]
        if self.reason_code != expected_reason:
            raise ValueError("Production World repair reason does not match its operation")
        allowed = {
            "revise_production_entity": self.closure.entity_keys + self.closure.state_keys,
            "rebind_scene_occurrence": self.closure.scene_scope_keys + self.closure.occurrence_keys,
            "revise_interaction": self.closure.interaction_keys,
            "revise_continuity": self.closure.continuity_keys,
        }[self.change_spec.operation]
        if any(value not in allowed for value in self.change_spec.target_keys):
            raise ValueError("Production World repair target is outside its closure")
        return self

    def validate_for(self, stage_key: SceneAnalysisStageKey) -> None:
        order = {
            "derive_production_entities": 1,
            "bind_scene_occurrences": 2,
            "reconcile_interaction_continuity": 3,
        }
        root = {
            "revise_production_entity": "derive_production_entities",
            "rebind_scene_occurrence": "bind_scene_occurrences",
            "revise_interaction": "reconcile_interaction_continuity",
            "revise_continuity": "reconcile_interaction_continuity",
        }[self.change_spec.operation]
        expected_type = {
            "derive_production_entities": "production_entity_fragment_candidate",
            "bind_scene_occurrences": "scene_binding_fragment_candidate",
            "reconcile_interaction_continuity": "continuity_fragment_candidate",
        }.get(stage_key)
        if (
            stage_key not in order
            or order[stage_key] < order[root]
            or self.base_candidate.identity.stage_key != stage_key
            or self.base_candidate.identity.shard_key != "script:full"
            or self.base_candidate.candidate_type != expected_type
        ):
            raise ValueError("Production World repair directive targets another stage")

    def validate_candidate_for(
        self,
        stage_key: SceneAnalysisStageKey,
        candidate: dict[str, Any],
    ) -> None:
        self.validate_for(stage_key)
        base = self.base_candidate.candidate
        if base == candidate:
            raise ValueError("Production World repair Candidate did not change")
        valid = False
        if stage_key == "derive_production_entities":
            valid = _preserves_production_entities(self, base, candidate)
        elif stage_key == "bind_scene_occurrences":
            valid = _preserves_scene_occurrences(self, base, candidate)
        elif stage_key == "reconcile_interaction_continuity":
            valid = _preserves_interaction_continuity(self, base, candidate)
        if not valid:
            raise ValueError(
                "Production World repair Candidate changed content outside its authorized closure"
            )


def _preserves_production_entities(
    directive: ProductionWorldRepairDirective,
    base: dict[str, Any],
    current: dict[str, Any],
) -> bool:
    if directive.change_spec.operation != "revise_production_entity" or not _same_except(
        base, current, "entities", "design_gaps", "review_issues"
    ):
        return False
    base_entities = _indexed_objects(base, "entities", "identity_key")
    current_entities = _indexed_objects(current, "entities", "identity_key")
    if (
        base_entities is None
        or current_entities is None
        or base_entities.keys() != current_entities.keys()
    ):
        return False
    targets = set(directive.change_spec.target_keys)
    authorized_gap_subjects: set[str] = set()
    for identity_key, base_entity in base_entities.items():
        current_entity = current_entities[identity_key]
        base_states = _indexed_objects(base_entity, "states", "state_key")
        current_states = _indexed_objects(current_entity, "states", "state_key")
        if (
            base_states is None
            or current_states is None
            or base_states.keys() != current_states.keys()
        ):
            return False
        if identity_key in targets:
            if not _same_fields(
                base_entity, current_entity, "identity_key", "kind", "specification_key"
            ):
                return False
            authorized_gap_subjects.update(
                {identity_key, str(base_entity.get("specification_key", "")), *base_states.keys()}
            )
            continue
        if not _same_except(base_entity, current_entity, "states"):
            return False
        for state_key, base_state in base_states.items():
            if state_key in targets:
                authorized_gap_subjects.add(state_key)
            elif base_state != current_states[state_key]:
                return False
    return _preserves_authorized_objects(
        base,
        current,
        "design_gaps",
        "gap_key",
        lambda value: value.get("subject_key") in authorized_gap_subjects,
    ) and _preserves_review_issues(directive, "derive_production_entities", base, current)


def _preserves_scene_occurrences(
    directive: ProductionWorldRepairDirective,
    base: dict[str, Any],
    current: dict[str, Any],
) -> bool:
    if directive.change_spec.operation not in {
        "revise_production_entity",
        "rebind_scene_occurrence",
    } or not _same_except(base, current, "scenes", "review_issues"):
        return False
    base_scenes = _indexed_objects(base, "scenes", "scene_scope_key")
    current_scenes = _indexed_objects(current, "scenes", "scene_scope_key")
    if base_scenes is None or current_scenes is None or base_scenes.keys() != current_scenes.keys():
        return False
    targets = set(directive.change_spec.target_keys)
    closure_occurrences = set(directive.closure.occurrence_keys)
    for scene_key, base_scene in base_scenes.items():
        current_scene = current_scenes[scene_key]
        if not _same_except(base_scene, current_scene, "occurrences"):
            return False
        base_occurrences = _indexed_objects(base_scene, "occurrences", "occurrence_key")
        current_occurrences = _indexed_objects(current_scene, "occurrences", "occurrence_key")
        if (
            base_occurrences is None
            or current_occurrences is None
            or base_occurrences.keys() != current_occurrences.keys()
        ):
            return False
        for occurrence_key, base_occurrence in base_occurrences.items():
            allowed = occurrence_key in closure_occurrences
            if directive.change_spec.operation == "rebind_scene_occurrence":
                allowed = scene_key in targets or occurrence_key in targets
            if not allowed and base_occurrence != current_occurrences[occurrence_key]:
                return False
    return _preserves_review_issues(directive, "bind_scene_occurrences", base, current)


def _preserves_interaction_continuity(
    directive: ProductionWorldRepairDirective,
    base: dict[str, Any],
    current: dict[str, Any],
) -> bool:
    if not _same_except(
        base,
        current,
        "interactions",
        "continuity",
        "continuity_ledger",
        "review_issues",
    ):
        return False
    operation = directive.change_spec.operation
    if operation in {"revise_production_entity", "rebind_scene_occurrence"}:
        allowed_interactions = set(directive.closure.interaction_keys)
        allowed_continuity = set(directive.closure.continuity_keys)
    elif operation == "revise_interaction":
        allowed_interactions = set(directive.change_spec.target_keys)
        allowed_continuity = set(directive.closure.continuity_keys)
    elif operation == "revise_continuity":
        allowed_interactions = set()
        allowed_continuity = set(directive.change_spec.target_keys)
    else:
        return False
    return (
        _preserves_fixed_keys(
            base, current, "interactions", "interaction_key", allowed_interactions
        )
        and _preserves_fixed_keys(base, current, "continuity", "continuity_key", allowed_continuity)
        and _preserves_fixed_keys(
            base,
            current,
            "continuity_ledger",
            "ledger_key",
            set(directive.closure.ledger_keys),
        )
        and _preserves_review_issues(directive, "reconcile_interaction_continuity", base, current)
    )


def _indexed_objects(
    root: dict[str, Any], field: str, key_field: str
) -> dict[str, dict[str, Any]] | None:
    values = root.get(field)
    if not isinstance(values, list):
        return None
    result: dict[str, dict[str, Any]] = {}
    for value in values:
        if not isinstance(value, dict):
            return None
        key = value.get(key_field)
        if not isinstance(key, str) or not key.strip() or key in result:
            return None
        result[key] = value
    return result


def _same_except(left: dict[str, Any], right: dict[str, Any], *fields: str) -> bool:
    return ({key: value for key, value in left.items() if key not in fields}) == {
        key: value for key, value in right.items() if key not in fields
    }


def _same_fields(left: dict[str, Any], right: dict[str, Any], *fields: str) -> bool:
    return all(left.get(field) == right.get(field) for field in fields)


def _preserves_fixed_keys(
    base: dict[str, Any],
    current: dict[str, Any],
    field: str,
    key_field: str,
    allowed: set[str],
) -> bool:
    base_items = _indexed_objects(base, field, key_field)
    current_items = _indexed_objects(current, field, key_field)
    if base_items is None or current_items is None or base_items.keys() != current_items.keys():
        return False
    return all(key in allowed or value == current_items[key] for key, value in base_items.items())


def _preserves_authorized_objects(
    base: dict[str, Any],
    current: dict[str, Any],
    field: str,
    key_field: str,
    authorized: Callable[[dict[str, Any]], bool],
) -> bool:
    base_items = _indexed_objects(base, field, key_field)
    current_items = _indexed_objects(current, field, key_field)
    if base_items is None or current_items is None:
        return False
    for key in base_items.keys() | current_items.keys():
        left = base_items.get(key)
        right = current_items.get(key)
        if left == right:
            continue
        if (left is not None and not authorized(left)) or (
            right is not None and not authorized(right)
        ):
            return False
    return True


def _preserves_review_issues(
    directive: ProductionWorldRepairDirective,
    stage: str,
    base: dict[str, Any],
    current: dict[str, Any],
) -> bool:
    prefix = f"{stage}/"
    allowed = {
        reference.removeprefix(prefix)
        for reference in directive.issue_refs
        if reference.startswith(prefix)
    }
    return _preserves_authorized_objects(
        base,
        current,
        "review_issues",
        "issue_key",
        lambda value: value.get("issue_key") in allowed,
    )


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


class FrozenStructureIdentityCandidateRef(StrictSceneAnalysisModel):
    stage_key: str = Field(min_length=1)
    shard_key: str = Field(min_length=1)
    candidate_revision_id: UUID
    candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    source_invocation_id: UUID
    source_result_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    skill_release_id: UUID
    skill_release_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    stage_release_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    bundle_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    agent_image_digest: str = Field(pattern=r"^sha256:[0-9a-f]{64}$")


class FrozenEpisodeRef(StrictSceneAnalysisModel):
    temporary_episode_id: str = Field(pattern=r"^episode_[a-z0-9_]{1,80}$")
    episode_id: UUID
    episode_revision: int = Field(ge=1)
    position: int = Field(ge=1)
    script_version_id: UUID
    script_version: int = Field(ge=1)
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class FrozenStructureIdentitySceneRef(StrictSceneAnalysisModel):
    temporary_episode_id: str = Field(pattern=r"^episode_[a-z0-9_]{1,80}$")
    episode_id: UUID
    temporary_span_id: str = Field(pattern=r"^span_[a-z0-9_]{1,80}$")
    temporary_scene_id: str = Field(pattern=r"^scene_[a-z0-9_]{1,80}$")
    scene_owner_logical_id: UUID
    scope_key: str = Field(pattern=r"^scene:[0-9a-f-]{36}$")
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    evidence_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class FrozenStructureIdentity(StrictSceneAnalysisModel):
    temporary_identity_key: str = Field(
        pattern=r"^identity_(character|location|prop)_[a-z0-9_]{1,80}$"
    )
    identity_key: str = Field(min_length=1)
    kind: Literal["character", "location", "prop"]
    resolution: Literal["new", "reuse"]
    reuse_identity_key: str | None
    canonical_name: str = Field(min_length=1)
    aliases: list[str] = Field(min_length=1)


class FrozenStructureIdentityMentionMapping(StrictSceneAnalysisModel):
    kind: Literal["character", "location", "prop"]
    occurrence_role: Literal["actual", "mentioned_only"]
    temporary_scene_id: str = Field(pattern=r"^scene_[a-z0-9_]{1,80}$")
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    text_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    exact_anchor: str = Field(min_length=1)
    resolution: Literal["resolved", "unresolved"]
    identity_key: str | None


class FrozenStructureIdentityCoverage(StrictSceneAnalysisModel):
    scene_count: int = Field(ge=1)
    identity_count: int = Field(ge=1)
    mention_count: int = Field(ge=0)
    resolved_count: int = Field(ge=0)
    unresolved_count: int = Field(ge=0)
    mention_universe_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scope_set_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class FrozenStructureIdentitySet(StrictSceneAnalysisModel):
    schema_version: Literal["structure-identity-set-production"]
    id: UUID
    workspace_id: UUID
    project_id: UUID
    version: int = Field(ge=1)
    parent_version_id: UUID | None
    gate_input_id: UUID
    gate_input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    review_decision_id: UUID
    project_episode_receipt_id: UUID
    document_revision_id: UUID
    span_index_id: UUID
    candidate_refs: list[FrozenStructureIdentityCandidateRef]
    episode_refs: list[FrozenEpisodeRef] = Field(min_length=1)
    scene_refs: list[FrozenStructureIdentitySceneRef] = Field(min_length=1)
    identities: list[FrozenStructureIdentity] = Field(min_length=1)
    mention_mappings: list[FrozenStructureIdentityMentionMapping]
    coverage: FrozenStructureIdentityCoverage
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    created_by: UUID
    created_at: datetime

    @model_validator(mode="after")
    def validate_identity_set(self) -> FrozenStructureIdentitySet:
        identity_keys = [value.identity_key for value in self.identities]
        scene_keys = [value.scope_key for value in self.scene_refs]
        if len(identity_keys) != len(set(identity_keys)) or len(scene_keys) != len(set(scene_keys)):
            raise ValueError("frozen StructureIdentitySet contains duplicate identities or Scenes")
        identity_kinds = {value.identity_key: value.kind for value in self.identities}
        scene_ids = {value.temporary_scene_id for value in self.scene_refs}
        for value in self.mention_mappings:
            if value.temporary_scene_id not in scene_ids:
                raise ValueError("frozen identity mention references an unknown Scene")
            if (value.resolution == "resolved") != (value.identity_key is not None):
                raise ValueError("frozen identity mention resolution is inconsistent")
            if (
                value.identity_key is not None
                and identity_kinds.get(value.identity_key) != value.kind
            ):
                raise ValueError("frozen identity mention references a different identity kind")
            if value.kind == "location" and value.occurrence_role != "actual":
                raise ValueError("formal Scene location must be an actual occurrence")
        if (
            self.coverage.scene_count != len(self.scene_refs)
            or self.coverage.identity_count != len(self.identities)
            or self.coverage.mention_count != len(self.mention_mappings)
            or self.coverage.resolved_count + self.coverage.unresolved_count
            != self.coverage.mention_count
        ):
            raise ValueError("frozen StructureIdentitySet coverage is invalid")
        return self


class ProductionEntityDerivationInput(StrictSceneAnalysisModel):
    source_version_id: UUID
    source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    normalized_text: str = Field(min_length=1)
    structure_identity_set_version_id: UUID
    structure_identity_set_version_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    structure_identity_set: FrozenStructureIdentitySet
    scene_fact_candidate_revision_id: UUID
    scene_fact_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_fact_candidate: dict[str, Any]

    @model_validator(mode="after")
    def validate_frozen_inputs(self) -> ProductionEntityDerivationInput:
        from app.modules.storygraph.scene_analysis_candidates import SceneFactCandidate

        facts = SceneFactCandidate.model_validate(self.scene_fact_candidate)
        if (
            hashlib.sha256(self.normalized_text.encode("utf-8")).hexdigest() != self.source_hash
            or facts.source_version_id != self.source_version_id
            or facts.source_hash != self.source_hash
            or self.structure_identity_set.id != self.structure_identity_set_version_id
            or self.structure_identity_set.content_hash != self.structure_identity_set_version_hash
            or self.structure_identity_set.document_revision_id != self.source_version_id
        ):
            raise ValueError("production entity frozen input lineage drifted")
        expected_scenes = {
            scene.temporary_scene_id: (scene.source_start, scene.source_end)
            for scene in facts.scenes
        }
        supplied_scenes = {
            scene.temporary_scene_id: (scene.source_start, scene.source_end)
            for scene in self.structure_identity_set.scene_refs
        }
        if supplied_scenes != expected_scenes:
            raise ValueError("formal StructureIdentitySet does not cover the frozen SceneFacts")
        for evidence in self.scene_fact_evidence():
            evidence.validate_for_text(self.normalized_text)
        return self

    def scene_fact_evidence(self) -> list[SourceEvidenceSpan]:
        from app.modules.storygraph.scene_analysis_candidates import SceneFactCandidate

        facts = SceneFactCandidate.model_validate(self.scene_fact_candidate)
        evidence: list[SourceEvidenceSpan] = []
        for scene in facts.scenes:
            if scene.location is not None:
                evidence.append(scene.location.evidence)
            if scene.time is not None:
                evidence.append(scene.time.evidence)
            evidence.extend(value.evidence for value in scene.actions)
            evidence.extend(value.evidence for value in scene.dialogues)
            evidence.extend(value.evidence for value in scene.raw_character_mentions)
            evidence.extend(value.evidence for value in scene.raw_prop_mentions)
        return evidence

    def scene_fact_evidence_universe(self) -> set[tuple[object, ...]]:
        return {
            (item.source_start, item.source_end, item.text_hash, item.exact_anchor)
            for item in self.scene_fact_evidence()
        }


class SceneOccurrenceBindingInput(ProductionEntityDerivationInput):
    production_entity_candidate_revision_id: UUID
    production_entity_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    production_entity_candidate: dict[str, Any]

    @model_validator(mode="after")
    def validate_production_entities(self) -> SceneOccurrenceBindingInput:
        from app.modules.storygraph.scene_analysis_candidates import (
            ProductionEntityFragmentCandidate,
        )

        candidate = ProductionEntityFragmentCandidate.model_validate(
            self.production_entity_candidate
        )
        candidate.validate_for(self)
        return self


class InteractionContinuityInput(SceneOccurrenceBindingInput):
    scene_binding_candidate_revision_id: UUID
    scene_binding_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    scene_binding_candidate: dict[str, Any]

    @model_validator(mode="after")
    def validate_scene_bindings(self) -> InteractionContinuityInput:
        from app.modules.storygraph.scene_analysis_candidates import (
            SceneBindingFragmentCandidate,
        )

        candidate = SceneBindingFragmentCandidate.model_validate(self.scene_binding_candidate)
        candidate.validate_for_input(self)
        return self


class SceneAnalysisPayload(StrictSceneAnalysisModel):
    variant: SceneAnalysisStageVariant
    scope: SceneAnalysisScope
    source_refs: list[ScriptSourceVersionIdentity] = Field(min_length=1, max_length=1)
    upstream_candidates: list[SceneAnalysisCandidateRevisionIdentity]
    shard: SceneAnalysisShard
    stage_input: dict[str, Any]
    production_world_repair: ProductionWorldRepairDirective | None = None

    @model_validator(mode="after")
    def validate_stage_input(self) -> SceneAnalysisPayload:
        source = self.source_refs[0]
        if self.production_world_repair is not None:
            self.production_world_repair.validate_for(self.variant.stage_key)
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
        elif self.variant.stage_key == "derive_production_entities":
            value = ProductionEntityDerivationInput.model_validate(self.stage_input)
            if (
                len(self.upstream_candidates) != 1
                or self.upstream_candidates[0].stage_key != "extract_scene_facts"
                or source.version_id != value.source_version_id
                or source.content_hash != value.source_hash
                or self.upstream_candidates[0].candidate_revision_id
                != value.scene_fact_candidate_revision_id
                or self.upstream_candidates[0].candidate_revision_hash
                != value.scene_fact_candidate_revision_hash
                or self.scope.workspace_id != value.structure_identity_set.workspace_id
                or self.scope.project_id != value.structure_identity_set.project_id
                or self.shard.codepoint_start != 0
                or self.shard.codepoint_end != len(value.normalized_text)
            ):
                raise ValueError(
                    "production entity input does not match its frozen formal identities"
                )
        elif self.variant.stage_key == "bind_scene_occurrences":
            value = SceneOccurrenceBindingInput.model_validate(self.stage_input)
            expected = {
                "extract_scene_facts": (
                    value.scene_fact_candidate_revision_id,
                    value.scene_fact_candidate_revision_hash,
                ),
                "derive_production_entities": (
                    value.production_entity_candidate_revision_id,
                    value.production_entity_candidate_revision_hash,
                ),
            }
            supplied = {
                item.stage_key: (item.candidate_revision_id, item.candidate_revision_hash)
                for item in self.upstream_candidates
            }
            if (
                len(self.upstream_candidates) != 2
                or supplied != expected
                or source.version_id != value.source_version_id
                or source.content_hash != value.source_hash
                or self.scope.workspace_id != value.structure_identity_set.workspace_id
                or self.scope.project_id != value.structure_identity_set.project_id
                or self.shard.codepoint_start != 0
                or self.shard.codepoint_end != len(value.normalized_text)
            ):
                raise ValueError("Scene binding input does not match its frozen production graph")
        elif self.variant.stage_key == "reconcile_interaction_continuity":
            value = InteractionContinuityInput.model_validate(self.stage_input)
            expected = {
                "extract_scene_facts": (
                    value.scene_fact_candidate_revision_id,
                    value.scene_fact_candidate_revision_hash,
                ),
                "derive_production_entities": (
                    value.production_entity_candidate_revision_id,
                    value.production_entity_candidate_revision_hash,
                ),
                "bind_scene_occurrences": (
                    value.scene_binding_candidate_revision_id,
                    value.scene_binding_candidate_revision_hash,
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
                or self.scope.workspace_id != value.structure_identity_set.workspace_id
                or self.scope.project_id != value.structure_identity_set.project_id
                or self.shard.codepoint_start != 0
                or self.shard.codepoint_end != len(value.normalized_text)
            ):
                raise ValueError(
                    "interaction continuity input does not match its frozen production graph"
                )
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
        "production_entity_fragment_candidate",
        "scene_binding_fragment_candidate",
        "continuity_fragment_candidate",
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
    if value.get("production_world_repair") is None:
        value.pop("production_world_repair", None)
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
