from __future__ import annotations

import hashlib
import re
from typing import Any, Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.candidate_runtime.canonical import canonical_hash
from app.modules.storygraph.candidate_schemas import (
    BIBLE_DETERMINISTIC_GATE_CODES,
    BIBLE_REPAIR_FIELD_TYPES,
    CandidateIssue,
    Evidence,
    SourceEvidenceCandidate,
    StoryAnalysisCandidate,
    StoryReconciliationCandidate,
)

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


class SourceEvidenceEpisodeMarkerHint(BaseModel):
    model_config = ConfigDict(extra="forbid")

    episode_number: int = Field(ge=1)
    label: str = Field(min_length=1)
    absolute_start: int = Field(ge=0)
    absolute_end: int = Field(ge=1)

    @model_validator(mode="after")
    def validate_range(self) -> SourceEvidenceEpisodeMarkerHint:
        if self.absolute_end <= self.absolute_start:
            raise ValueError("Source Evidence marker range must be increasing")
        return self


class SourceEvidenceStageInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    document_revision_id: UUID
    normalized_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    logical_source_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    logical_start: int = Field(ge=0)
    logical_end: int = Field(ge=1)
    context_start: int = Field(ge=0)
    context_end: int = Field(ge=1)
    normalized_text: str = Field(min_length=1)
    episode_marker_hints: list[SourceEvidenceEpisodeMarkerHint]

    @model_validator(mode="after")
    def validate_ranges(self) -> SourceEvidenceStageInput:
        if (
            self.logical_end <= self.logical_start
            or self.context_start > self.logical_start
            or self.context_end < self.logical_end
            or self.context_end - self.context_start != len(self.normalized_text)
        ):
            raise ValueError("Source Evidence stage ranges do not match its context")
        for marker in self.episode_marker_hints:
            if marker.absolute_start < self.context_start or marker.absolute_end > self.context_end:
                raise ValueError("Source Evidence marker is outside its context")
        relative_start = self.logical_start - self.context_start
        relative_end = self.logical_end - self.context_start
        logical_text = self.normalized_text[relative_start:relative_end]
        if hashlib.sha256(logical_text.encode("utf-8")).hexdigest() != self.logical_source_hash:
            raise ValueError("Source Evidence logical source hash does not match its text")
        return self


class StoryAnalysisStageInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    evidence_shard_key: str = Field(min_length=1)
    evidence_candidate_revision_id: UUID
    evidence_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    logical_start: int = Field(ge=0)
    logical_end: int = Field(ge=1)
    candidate_item_start: int = Field(ge=0)
    candidate_item_end: int = Field(ge=0)
    evidence_candidate: SourceEvidenceCandidate

    @model_validator(mode="after")
    def validate_range(self) -> StoryAnalysisStageInput:
        item_count = len(self.evidence_candidate.observations) + len(
            self.evidence_candidate.review_issues
        )
        if (
            self.logical_end <= self.logical_start
            or self.candidate_item_end < self.candidate_item_start
            or self.candidate_item_end - self.candidate_item_start != item_count
        ):
            raise ValueError("Story analysis ranges do not match the exact candidate partition")
        return self


class EpisodeSegmentationEvidenceLeaf(BaseModel):
    model_config = ConfigDict(extra="forbid")

    shard_key: str = Field(min_length=1)
    candidate_revision_id: UUID
    candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class EpisodeSegmentationMarkerHint(BaseModel):
    model_config = ConfigDict(extra="forbid")

    episode_number: int = Field(ge=1)
    label: str = Field(min_length=1)
    evidence: Evidence

    @model_validator(mode="after")
    def validate_episode_number(self) -> EpisodeSegmentationMarkerHint:
        if self.evidence.episode_number != self.episode_number:
            raise ValueError("Episode marker number does not match its Evidence")
        return self


class EpisodeSegmentationEvidenceIndexItem(BaseModel):
    model_config = ConfigDict(extra="forbid")

    index_key: str = Field(min_length=1)
    kind: Literal["marker", "event", "evidence"]
    label: str = Field(min_length=1)
    shard_key: str = Field(min_length=1)
    candidate_revision_id: UUID
    candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    evidence: Evidence


class EpisodeSegmentationStageInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    document_revision_id: UUID
    normalized_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    source_code_points: int = Field(ge=1)
    target_duration_ms: int = Field(ge=1000, le=7_200_000)
    bible_version_id: UUID
    bible_version: int = Field(ge=1)
    bible_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    materialization_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    evidence_aggregate_revision_id: UUID
    evidence_aggregate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    evidence_leaves: list[EpisodeSegmentationEvidenceLeaf] = Field(min_length=1)
    marker_hints: list[EpisodeSegmentationMarkerHint]
    evidence_index: list[EpisodeSegmentationEvidenceIndexItem] = Field(min_length=1, max_length=512)

    @model_validator(mode="after")
    def validate_bounded_evidence(self) -> EpisodeSegmentationStageInput:
        shard_keys = [value.shard_key for value in self.evidence_leaves]
        leaf_keys = [
            (value.shard_key, value.candidate_revision_id, value.candidate_revision_hash)
            for value in self.evidence_leaves
        ]
        if shard_keys != sorted(set(shard_keys)) or len(leaf_keys) != len(set(leaf_keys)):
            raise ValueError("Episode segmentation Evidence leaves must be unique and sorted")
        allowed_leaves = set(leaf_keys)
        index_keys: set[str] = set()
        marker_evidence: set[tuple[int, int, str, str, int | None]] = set()
        for item in self.evidence_index:
            leaf_key = (
                item.shard_key,
                item.candidate_revision_id,
                item.candidate_revision_hash,
            )
            if (
                leaf_key not in allowed_leaves
                or item.index_key in index_keys
                or item.evidence.source_end > self.source_code_points
            ):
                raise ValueError("Episode segmentation Evidence index is invalid")
            index_keys.add(item.index_key)
            if item.kind == "marker":
                marker_evidence.add(_episode_segmentation_evidence_key(item.evidence))
        marker_starts: set[int] = set()
        for marker in self.marker_hints:
            if (
                marker.evidence.source_start in marker_starts
                or marker.evidence.source_end > self.source_code_points
                or _episode_segmentation_evidence_key(marker.evidence) not in marker_evidence
            ):
                raise ValueError("Episode segmentation marker is not in its bounded index")
            marker_starts.add(marker.evidence.source_start)
        return self


def _episode_segmentation_evidence_key(
    value: Evidence,
) -> tuple[int, int, str, str, int | None]:
    return (
        value.source_start,
        value.source_end,
        value.text_hash,
        value.exact_anchor,
        value.episode_number,
    )


class StoryReconciliationInputCandidate(BaseModel):
    model_config = ConfigDict(extra="forbid")

    shard_key: str = Field(min_length=1)
    candidate_revision_id: UUID
    candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    candidate_item_start: int | None = Field(default=None, ge=0)
    candidate_item_end: int | None = Field(default=None, ge=0)
    candidate: dict[str, Any]

    @model_validator(mode="after")
    def validate_candidate_range(self) -> StoryReconciliationInputCandidate:
        if (self.candidate_item_start is None) != (self.candidate_item_end is None):
            raise ValueError("Story reconcile candidate range must be complete")
        if (
            self.candidate_item_start is not None
            and self.candidate_item_end is not None
            and self.candidate_item_end <= self.candidate_item_start
        ):
            raise ValueError("Story reconcile candidate range must be increasing")
        return self


class StoryReconciliationStageInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    level: int = Field(ge=0)
    candidate_type: Literal["story_analysis_candidate", "story_reconciliation_candidate"]
    candidates: list[StoryReconciliationInputCandidate] = Field(min_length=1, max_length=2)

    @model_validator(mode="after")
    def validate_candidates(self) -> StoryReconciliationStageInput:
        model: type[BaseModel] = (
            StoryAnalysisCandidate
            if self.candidate_type == "story_analysis_candidate"
            else StoryReconciliationCandidate
        )
        identities: set[tuple[UUID, str]] = set()
        for value in self.candidates:
            candidate = model.model_validate(value.candidate)
            if isinstance(candidate, StoryAnalysisCandidate):
                item_count = (
                    len(candidate.entities)
                    + len(candidate.world_entries)
                    + len(candidate.claims)
                    + len(candidate.arcs)
                    + len(candidate.review_issues)
                )
            else:
                item_count = (
                    len(candidate.canonical_entities)
                    + len(candidate.canonical_world_entries)
                    + len(candidate.merged_claims)
                    + len(candidate.merged_arcs)
                    + len(candidate.conflicts)
                    + len(candidate.review_issues)
                )
            if (
                value.candidate_item_start is not None
                and value.candidate_item_end is not None
                and value.candidate_item_end - value.candidate_item_start != item_count
            ):
                raise ValueError(
                    "Story reconcile candidate range does not match its exact partition"
                )
            identity = (value.candidate_revision_id, value.candidate_revision_hash)
            if identity in identities:
                raise ValueError("Story reconcile candidates must be unique")
            identities.add(identity)
        return self


class StoryGraphGateBlocker(BaseModel):
    model_config = ConfigDict(extra="forbid")

    code: str = Field(min_length=1)
    subject_key: str = Field(min_length=1)
    related_key: str | None = None
    summary: str = Field(min_length=1)


class StoryGraphDeterministicGateResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    gate_version: Literal["bible-deterministic-gate-v1"]
    target_candidate_revision_id: UUID
    target_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    blockers: list[StoryGraphGateBlocker]

    @model_validator(mode="after")
    def validate_blocker_order(self) -> StoryGraphDeterministicGateResult:
        identities = [
            (value.code, value.subject_key, value.related_key or "") for value in self.blockers
        ]
        if identities != sorted(set(identities)):
            raise ValueError("deterministic gate blockers must be unique and sorted")
        return self


class StoryGraphReviewStageInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    reviewed_stage: Literal["reconcile_story"]
    target_candidate_revision_id: UUID
    target_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    candidate_item_start: int = Field(ge=0)
    candidate_item_end: int = Field(ge=1)
    target_candidate: StoryReconciliationCandidate
    deterministic_gate: StoryGraphDeterministicGateResult

    @model_validator(mode="after")
    def validate_target(self) -> StoryGraphReviewStageInput:
        item_count = (
            len(self.target_candidate.canonical_entities)
            + len(self.target_candidate.canonical_world_entries)
            + len(self.target_candidate.merged_claims)
            + len(self.target_candidate.merged_arcs)
            + len(self.target_candidate.conflicts)
            + len(self.target_candidate.review_issues)
        )
        if (
            self.candidate_item_end <= self.candidate_item_start
            or self.candidate_item_end - self.candidate_item_start != item_count
            or self.deterministic_gate.target_candidate_revision_id
            != self.target_candidate_revision_id
            or self.deterministic_gate.target_candidate_revision_hash
            != self.target_candidate_revision_hash
        ):
            raise ValueError("StoryGraph review input does not match its frozen target")
        return self


class StoryGraphRepairAllowedTarget(BaseModel):
    model_config = ConfigDict(extra="forbid")

    candidate_key: str = Field(min_length=1)
    allowed_fields: list[str] = Field(min_length=1, max_length=32)
    base_fragment_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    fragment: dict[str, Any]

    @model_validator(mode="after")
    def validate_fragment(self) -> StoryGraphRepairAllowedTarget:
        if (
            self.allowed_fields != sorted(set(self.allowed_fields))
            or any(
                re.fullmatch(r"[a-z][a-z0-9_]{0,79}", value) is None
                or value not in BIBLE_REPAIR_FIELD_TYPES
                for value in self.allowed_fields
            )
            or canonical_hash(self.fragment) != self.base_fragment_hash
        ):
            raise ValueError("invalid StoryGraph repair allowed target")
        return self


class StoryGraphRepairReadOnlyFragment(BaseModel):
    model_config = ConfigDict(extra="forbid")

    candidate_key: str = Field(min_length=1)
    fragment_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    fragment: dict[str, Any]

    @model_validator(mode="after")
    def validate_fragment(self) -> StoryGraphRepairReadOnlyFragment:
        if canonical_hash(self.fragment) != self.fragment_hash:
            raise ValueError("StoryGraph repair read-only fragment hash mismatch")
        return self


class StoryGraphRepairStageInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    target_candidate_revision_id: UUID
    target_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    review_candidate_revision_id: UUID
    review_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    target_issue: CandidateIssue
    allowed_targets: list[StoryGraphRepairAllowedTarget] = Field(min_length=1, max_length=32)
    read_only_adjacency: list[StoryGraphRepairReadOnlyFragment] = Field(max_length=64)
    repair_round: int = Field(ge=1, le=3)
    max_repair_rounds: int = Field(ge=1, le=3)

    @model_validator(mode="after")
    def validate_boundary(self) -> StoryGraphRepairStageInput:
        allowed_keys = [value.candidate_key for value in self.allowed_targets]
        adjacent_keys = [value.candidate_key for value in self.read_only_adjacency]
        if (
            self.target_issue.severity != "blocking"
            or self.target_issue.code in BIBLE_DETERMINISTIC_GATE_CODES
            or not self.target_issue.evidence
            or self.repair_round > self.max_repair_rounds
            or len(set((*allowed_keys, *adjacent_keys))) != len(allowed_keys) + len(adjacent_keys)
            or (
                self.target_issue.subject_key is not None
                and self.target_issue.subject_key not in allowed_keys
            )
        ):
            raise ValueError("invalid StoryGraph repair boundary")
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
        if self.stage == "extract_source_evidence":
            source_input = SourceEvidenceStageInput.model_validate(self.stage_input)
            if (
                len(self.source_refs) != 1
                or self.source_refs[0].owner_kind != "production/script"
                or self.source_refs[0].owner_version_id != source_input.document_revision_id
                or self.source_refs[0].content_hash != source_input.normalized_hash
                or self.upstream_candidates
                or self.base_storygraph_version_id is not None
                or self.shard.kind != "source_slice"
                or self.shard.absolute_start != source_input.logical_start
                or self.shard.absolute_end != source_input.logical_end
            ):
                raise ValueError(
                    "Source Evidence input does not match its immutable source and shard"
                )
        elif self.stage == "analyze_story":
            stage_input = StoryAnalysisStageInput.model_validate(self.stage_input)
            if (
                len(self.source_refs) != 1
                or len(self.upstream_candidates) != 1
                or self.upstream_candidates[0].stage != "extract_source_evidence"
                or self.upstream_candidates[0].shard_key != stage_input.evidence_shard_key
                or self.upstream_candidates[0].candidate_revision_id
                != stage_input.evidence_candidate_revision_id
                or self.upstream_candidates[0].candidate_revision_hash
                != stage_input.evidence_candidate_revision_hash
                or self.base_storygraph_version_id is not None
                or self.shard.kind != "story_map"
                or self.shard.absolute_start != stage_input.logical_start
                or self.shard.absolute_end != stage_input.logical_end
            ):
                raise ValueError("Story analysis input does not match its exact Evidence revision")
        elif self.stage == "reconcile_story":
            stage_input = StoryReconciliationStageInput.model_validate(self.stage_input)
            expected_stage = (
                "analyze_story"
                if stage_input.candidate_type == "story_analysis_candidate"
                else "reconcile_story"
            )
            upstream = {
                (value.shard_key, value.candidate_revision_id, value.candidate_revision_hash)
                for value in self.upstream_candidates
                if value.stage == expected_stage
            }
            supplied = {
                (value.shard_key, value.candidate_revision_id, value.candidate_revision_hash)
                for value in stage_input.candidates
            }
            if (
                len(self.source_refs) != 1
                or len(self.upstream_candidates) != len(stage_input.candidates)
                or upstream != supplied
                or self.base_storygraph_version_id is not None
                or self.shard.kind != "story_reduce"
                or self.shard.absolute_start is not None
                or self.shard.absolute_end is not None
            ):
                raise ValueError("Story reconcile input does not match its exact child revisions")
        elif self.stage == "segment_episodes":
            stage_input = EpisodeSegmentationStageInput.model_validate(self.stage_input)
            script_sources = [
                value
                for value in self.source_refs
                if value.owner_kind == "production/script"
                and value.owner_version_id == stage_input.document_revision_id
                and value.content_hash == stage_input.normalized_hash
            ]
            materialization_sources = [
                value
                for value in self.source_refs
                if value.owner_kind == "production/bible-materialization"
                and value.owner_logical_id == str(stage_input.bible_version_id)
                and value.owner_version_id == stage_input.bible_version_id
                and value.revision == stage_input.bible_version
                and value.content_hash == stage_input.materialization_hash
            ]
            supplied_leaves = {
                (value.shard_key, value.candidate_revision_id, value.candidate_revision_hash)
                for value in stage_input.evidence_leaves
            }
            upstream_leaves = {
                (value.shard_key, value.candidate_revision_id, value.candidate_revision_hash)
                for value in self.upstream_candidates
                if value.stage == "extract_source_evidence"
            }
            if (
                len(self.source_refs) != 2
                or len(script_sources) != 1
                or len(materialization_sources) != 1
                or len(self.upstream_candidates) != len(stage_input.evidence_leaves)
                or supplied_leaves != upstream_leaves
                or self.base_storygraph_version_id is not None
                or self.shard.kind != "episode_segmentation"
                or self.shard.absolute_start != 0
                or self.shard.absolute_end != stage_input.source_code_points
            ):
                raise ValueError(
                    "Episode segmentation input does not match its exact immutable sources"
                )
        elif self.stage == "review_storygraph":
            stage_input = StoryGraphReviewStageInput.model_validate(self.stage_input)
            if (
                len(self.source_refs) != 1
                or len(self.upstream_candidates) != 1
                or self.upstream_candidates[0].stage != stage_input.reviewed_stage
                or self.upstream_candidates[0].candidate_revision_id
                != stage_input.target_candidate_revision_id
                or self.upstream_candidates[0].candidate_revision_hash
                != stage_input.target_candidate_revision_hash
                or self.base_storygraph_version_id is not None
                or self.shard.kind != "story_review"
                or self.shard.absolute_start is not None
                or self.shard.absolute_end is not None
            ):
                raise ValueError(
                    "StoryGraph review input does not match its exact candidate revision"
                )
        elif self.stage == "repair_candidate":
            stage_input = StoryGraphRepairStageInput.model_validate(self.stage_input)
            expected = {
                (
                    "reconcile_story",
                    stage_input.target_candidate_revision_id,
                    stage_input.target_candidate_revision_hash,
                ),
                (
                    "review_storygraph",
                    stage_input.review_candidate_revision_id,
                    stage_input.review_candidate_revision_hash,
                ),
            }
            supplied = {
                (value.stage, value.candidate_revision_id, value.candidate_revision_hash)
                for value in self.upstream_candidates
            }
            if (
                len(self.source_refs) != 1
                or len(self.upstream_candidates) != 2
                or supplied != expected
                or self.base_storygraph_version_id is not None
                or self.shard.kind != "candidate_repair"
                or self.shard.absolute_start is not None
                or self.shard.absolute_end is not None
            ):
                raise ValueError(
                    "StoryGraph repair input does not match its exact candidate revisions"
                )
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


class Executor(BaseModel):
    model_config = ConfigDict(extra="forbid")

    name: str = Field(min_length=1)
    version: str = Field(min_length=1)
    model: str = Field(min_length=1)


class ResultError(BaseModel):
    model_config = ConfigDict(extra="forbid")

    code: str = Field(min_length=1)
    summary: str = Field(min_length=1)
    retryable: bool


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
        from app.modules.storygraph.skill_registry import stage_spec

        if (
            self.invocation_id != invocation.invocation_id
            or self.stage != invocation.payload.stage
            or self.shard_key != invocation.payload.shard_key
            or self.input_hash != invocation.input_hash
            or self.candidate_type != stage_spec(invocation.payload.stage).candidate_type
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
