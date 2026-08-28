from __future__ import annotations

import hashlib
from typing import Any, Literal, cast
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

BIBLE_REPAIR_FIELD_TYPES = {
    "aliases": "strings",
    "ambiguities": "strings",
    "anchor_keys": "strings",
    "canonical_name": "text",
    "category": "text",
    "claim_type": "text",
    "entity_keys": "strings",
    "facts": "strings",
    "label": "text",
    "normalized_name": "text",
    "participant_keys": "strings",
    "polarity": "text",
    "rules": "strings",
    "scope": "text",
    "status": "text",
    "summary": "text",
    "title": "text",
}

BIBLE_DETERMINISTIC_GATE_CODES = {
    "world_unknown_entity",
    "world_duplicate_entity",
    "claim_unknown_participant",
    "claim_duplicate_participant",
    "claim_unknown_anchor",
    "claim_duplicate_anchor",
}


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid")

    @model_validator(mode="after")
    def reject_business_write_shapes(self) -> StrictModel:
        forbidden = {
            "command",
            "commands",
            "sql",
            "graph",
            "graph_json",
            "storygraph_overwrite",
            "owner_write",
        }

        def visit(value: Any) -> None:
            if isinstance(value, dict):
                for key, item in cast(dict[str, Any], value).items():
                    if str(key).casefold() in forbidden:
                        raise ValueError("candidate contains a forbidden business write shape")
                    visit(item)
            elif isinstance(value, list):
                for item in cast(list[Any], value):
                    visit(item)

        visit(self.__dict__)
        return self


class Evidence(StrictModel):
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    text_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    exact_anchor: str = Field(min_length=1)
    episode_number: int | None = Field(ge=1)

    @model_validator(mode="after")
    def increasing_range(self) -> Evidence:
        if self.source_end <= self.source_start:
            raise ValueError("evidence range must be increasing")
        return self


class CandidateIssue(StrictModel):
    issue_key: str = Field(min_length=1)
    code: str = Field(min_length=1)
    severity: Literal["warning", "blocking"]
    scope: str = Field(min_length=1)
    subject_key: str | None
    summary: str = Field(min_length=1)
    repair_hint: str | None
    evidence: list[Evidence]


class Observation(StrictModel):
    observation_key: str = Field(min_length=1)
    kind: Literal["entity", "entity_state", "world_entry", "event", "marker"]
    proposed_key: str = Field(min_length=1)
    label: str = Field(min_length=1)
    facts: list[str]
    evidence: list[Evidence] = Field(min_length=1)
    ambiguities: list[str]


class SourceEvidenceCandidate(StrictModel):
    observations: list[Observation]
    review_issues: list[CandidateIssue]


class AssetSpec(StrictModel):
    kind: Literal["character", "location", "prop", "costume", "visual_style", "voice"]
    identity: str | None
    appearance: str | None
    age_impression: str | None
    temperament: list[str]
    goals: list[str]
    relationships: list[str]
    arc_summary: str | None
    voice_profile: str | None
    spatial_description: str | None
    time_weather: str | None
    visual_elements: list[str]
    lighting: str | None
    material: str | None
    usage_context: str | None
    visual_language: str | None
    palette: str | None
    lighting_language: str | None
    negative_constraints: list[str]
    source_kind: Literal["synthetic_recording", "human_recording", "voice_clone"] | None
    language: str | None
    performance_traits: list[str]
    allowed_usage: list[str]


class EntityState(StrictModel):
    state_key: str = Field(min_length=1)
    label: str = Field(min_length=1)
    state_spec: AssetSpec
    episode_numbers: list[int]
    evidence: list[Evidence] = Field(min_length=1)
    ambiguities: list[str]


class Entity(StrictModel):
    entity_key: str = Field(min_length=1)
    kind: Literal["character", "location", "prop", "costume", "visual_style", "voice"]
    canonical_name: str = Field(min_length=1)
    normalized_name: str = Field(min_length=1)
    aliases: list[str]
    stable_spec: AssetSpec
    episode_numbers: list[int]
    evidence: list[Evidence] = Field(min_length=1)
    states: list[EntityState]
    ambiguities: list[str]


class WorldEntry(StrictModel):
    entry_key: str = Field(min_length=1)
    category: str = Field(min_length=1)
    title: str = Field(min_length=1)
    facts: list[str]
    rules: list[str]
    entity_keys: list[str]
    episode_numbers: list[int]
    evidence: list[Evidence] = Field(min_length=1)
    ambiguities: list[str]


class ClaimCandidate(StrictModel):
    claim_key: str = Field(min_length=1)
    claim_type: Literal["relationship", "causal", "continuity", "foreshadowing"]
    participant_keys: list[str] = Field(min_length=1)
    anchor_keys: list[str] = Field(min_length=1)
    scope: str = Field(min_length=1)
    polarity: Literal["positive", "negative", "mixed", "unknown"]
    status: Literal["proposed", "ambiguous", "conflicted"]
    evidence: list[Evidence] = Field(min_length=1)


class StoryArcCandidate(StrictModel):
    arc_key: str = Field(min_length=1)
    title: str = Field(min_length=1)
    summary: str = Field(min_length=1)
    evidence: list[Evidence] = Field(min_length=1)


class StoryAnalysisCandidate(StrictModel):
    entities: list[Entity]
    world_entries: list[WorldEntry]
    claims: list[ClaimCandidate]
    arcs: list[StoryArcCandidate]
    review_issues: list[CandidateIssue]


class StoryReconciliationCandidate(StrictModel):
    canonical_entities: list[Entity]
    canonical_world_entries: list[WorldEntry]
    merged_claims: list[ClaimCandidate]
    merged_arcs: list[StoryArcCandidate]
    conflicts: list[CandidateIssue]
    review_issues: list[CandidateIssue]


class EpisodeBoundary(StrictModel):
    boundary_key: str = Field(min_length=1)
    episode_order: int = Field(ge=1)
    title: str = Field(min_length=1)
    absolute_start: int = Field(ge=0)
    absolute_end: int = Field(gt=0)
    evidence: list[Evidence] = Field(min_length=1)

    @model_validator(mode="after")
    def increasing_range(self) -> EpisodeBoundary:
        if self.absolute_end <= self.absolute_start:
            raise ValueError("episode boundary range must be increasing")
        return self


class EpisodeSegmentationCandidate(StrictModel):
    boundaries: list[EpisodeBoundary] = Field(min_length=1, max_length=1000)
    review_issues: list[CandidateIssue]

    @model_validator(mode="after")
    def validate_coverage_order(self) -> EpisodeSegmentationCandidate:
        keys: set[str] = set()
        previous_end = 0
        for index, boundary in enumerate(self.boundaries, start=1):
            if (
                boundary.boundary_key in keys
                or boundary.episode_order != index
                or boundary.absolute_start != previous_end
                or any(
                    evidence.source_start < boundary.absolute_start
                    or evidence.source_end > boundary.absolute_end
                    for evidence in boundary.evidence
                )
            ):
                raise ValueError(
                    "episode boundaries must be unique, ordered, contiguous, and evidenced"
                )
            keys.add(boundary.boundary_key)
            previous_end = boundary.absolute_end
        return self


class StructureAttributes(StrictModel):
    scene_key: str | None
    speaker_key: str | None
    participant_keys: list[str]
    location_key: str | None
    time_hint: str | None
    dialogue_text: str | None
    action: str | None
    occurrence_entity_key: str | None
    state_key: str | None
    continuity_notes: list[str]


class StructureFragment(StrictModel):
    temporary_key: str = Field(min_length=1)
    kind: Literal["scene", "dialogue", "beat", "occurrence"]
    source_keys: list[str] = Field(min_length=1)
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    summary: str = Field(min_length=1)
    evidence: list[Evidence] = Field(min_length=1)
    attributes: StructureAttributes

    @model_validator(mode="after")
    def increasing_range(self) -> StructureFragment:
        if self.source_end <= self.source_start:
            raise ValueError("structure fragment range must be increasing")
        return self


class EpisodeAnalysisCandidate(StrictModel):
    episode_id: UUID
    script_version_id: UUID
    logical_start: int = Field(ge=0)
    logical_end: int = Field(gt=0)
    fragments: list[StructureFragment]
    claims: list[ClaimCandidate]
    review_issues: list[CandidateIssue]

    @model_validator(mode="after")
    def validate_identity_and_order(self) -> EpisodeAnalysisCandidate:
        if self.logical_end <= self.logical_start:
            raise ValueError("episode analysis range must be increasing")
        _validate_episode_candidate_keys(self.fragments, self.claims, self.review_issues)
        return self

    def validate_for(self, stage_input: Any) -> None:
        if (
            self.episode_id != stage_input.episode_id
            or self.script_version_id != stage_input.script_version_id
            or self.logical_start != stage_input.logical_start
            or self.logical_end != stage_input.logical_end
        ):
            raise ValueError("Episode analysis candidate does not match its frozen slice")
        _validate_episode_candidate_content(
            self.fragments,
            self.claims,
            self.review_issues,
            stage_input,
            source_start=self.logical_start,
            source_end=self.logical_end,
            context_start=stage_input.context_start,
            context_text=stage_input.context_text,
        )


class EpisodeReconciliationCandidate(StrictModel):
    episode_id: UUID
    script_version_id: UUID
    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    ordered_fragments: list[StructureFragment]
    claims: list[ClaimCandidate]
    conflicts: list[CandidateIssue]
    review_issues: list[CandidateIssue]

    @model_validator(mode="after")
    def validate_identity_and_order(self) -> EpisodeReconciliationCandidate:
        if self.source_end <= self.source_start:
            raise ValueError("episode reconciliation range must be increasing")
        _validate_episode_candidate_keys(
            self.ordered_fragments,
            self.claims,
            [*self.conflicts, *self.review_issues],
        )
        return self

    def validate_for(self, stage_input: Any) -> None:
        if (
            self.episode_id != stage_input.episode_id
            or self.script_version_id != stage_input.script_version_id
            or self.source_start != stage_input.episode_source_start
            or self.source_end != stage_input.episode_source_end
        ):
            raise ValueError("Episode reconciliation candidate does not match its frozen Episode")
        child_fragments: dict[str, StructureFragment] = {}
        child_claims: dict[str, ClaimCandidate] = {}
        allowed_issue_evidence: set[tuple[int, int, str, str, int | None]] = set()
        for child in stage_input.parsed_candidates():
            fragments = (
                child.fragments
                if isinstance(child, EpisodeAnalysisCandidate)
                else child.ordered_fragments
            )
            for fragment in fragments:
                if fragment.temporary_key in child_fragments:
                    raise ValueError("Episode reconciliation children contain duplicate fragments")
                child_fragments[fragment.temporary_key] = fragment
                allowed_issue_evidence.update(_evidence_key(value) for value in fragment.evidence)
            for claim in child.claims:
                if claim.claim_key in child_claims:
                    raise ValueError("Episode reconciliation children contain duplicate claims")
                child_claims[claim.claim_key] = claim
                allowed_issue_evidence.update(_evidence_key(value) for value in claim.evidence)
            for issue in (*getattr(child, "conflicts", []), *child.review_issues):
                allowed_issue_evidence.update(_evidence_key(value) for value in issue.evidence)
        if {value.temporary_key for value in self.ordered_fragments} != set(child_fragments):
            raise ValueError(
                "Episode reconciliation candidate did not preserve its exact child set"
            )
        if {value.claim_key for value in self.claims} != set(child_claims):
            raise ValueError("Episode reconciliation candidate did not preserve its exact claims")
        _validate_episode_candidate_content(
            self.ordered_fragments,
            self.claims,
            [*self.conflicts, *self.review_issues],
            stage_input,
            source_start=self.source_start,
            source_end=self.source_end,
        )
        if any(
            _evidence_key(evidence) not in allowed_issue_evidence
            for issue in (*self.conflicts, *self.review_issues)
            for evidence in issue.evidence
        ):
            raise ValueError("Episode reconciliation issue Evidence is outside its exact children")


class ShotVisual(StrictModel):
    shot_size: str | None
    camera_angle: str | None
    camera_movement: str | None
    composition: str | None
    environment: str | None
    lighting: str | None
    subject_placement: str | None
    facing: str | None
    gaze: str | None


class ShotSpec(StrictModel):
    duration_ms: int = Field(ge=500, le=15000)
    narrative_role: str = Field(min_length=1)
    action_beats: list[str]
    first_frame: str = Field(min_length=1)
    last_frame: str = Field(min_length=1)
    keyframe_notes: list[str]
    continuity_in: str
    continuity_out: str
    visual: ShotVisual
    dialogue: str | None
    performance: str | None
    ambience: str | None
    sound_effects: list[str]


class AssetReference(StrictModel):
    asset_id: UUID
    asset_version_id: UUID
    state_key: str | None
    usage: str = Field(min_length=1)


class ArtifactReference(StrictModel):
    artifact_id: UUID
    lineage_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    view_role: Literal["front", "profile", "back", "environment", "prop", "texture", "other"]


class ShotBindingCandidate(StrictModel):
    asset_references: list[AssetReference] = Field(min_length=1)
    artifact_references: list[ArtifactReference] = Field(min_length=1)
    effective_style_snapshot_id: UUID
    effective_style_snapshot_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class DraftShot(StrictModel):
    proposal_key: str = Field(min_length=1)
    position: int = Field(gt=0)
    title: str = Field(min_length=1)
    narrative_unit_version_ids: list[UUID] = Field(min_length=1)
    spec: ShotSpec
    asset_references: list[AssetReference]
    risk_codes: list[str]


class StoryboardRowCandidate(StrictModel):
    shots: list[DraftShot] = Field(min_length=1, max_length=120)


class ShotDetail(StrictModel):
    proposal_key: str = Field(min_length=1)
    accepted_intent_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    exact_asset_version_ids: list[UUID] = Field(min_length=1)
    detail: ShotSpec
    binding_candidate: ShotBindingCandidate
    review_issues: list[CandidateIssue]


class ShotDetailCandidate(StrictModel):
    shots: list[ShotDetail] = Field(min_length=1)


class StoryGraphReviewCandidate(StrictModel):
    reviewed_stage: str = Field(min_length=1)
    target_candidate_revision_id: UUID
    target_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    review_issues: list[CandidateIssue]

    def validate_for(self, stage_input: Any) -> None:
        if (
            self.reviewed_stage != stage_input.reviewed_stage
            or self.target_candidate_revision_id != stage_input.target_candidate_revision_id
            or self.target_candidate_revision_hash != stage_input.target_candidate_revision_hash
        ):
            raise ValueError("StoryGraph review candidate does not match its frozen target")
        _validate_review_issues(
            self.review_issues,
            _candidate_evidence_keys(stage_input.target_candidate),
            require_evidence=True,
        )


class RepairReplacement(StrictModel):
    text: str | None
    integer: int | None
    flag: bool | None
    strings: list[str] | None

    @model_validator(mode="after")
    def exactly_one_value(self) -> RepairReplacement:
        if (
            sum(value is not None for value in (self.text, self.integer, self.flag, self.strings))
            != 1
        ):
            raise ValueError("repair replacement must contain exactly one typed value")
        return self


class RepairOperation(StrictModel):
    target_candidate_key: str = Field(min_length=1)
    base_fragment_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    field_name: str = Field(min_length=1)
    replacement: RepairReplacement


class CandidateRepairPatch(StrictModel):
    target_candidate_revision_id: UUID
    target_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    operations: list[RepairOperation] = Field(min_length=1)
    review_issues: list[CandidateIssue]

    def validate_for(self, stage_input: Any) -> None:
        if (
            self.target_candidate_revision_id != stage_input.target_candidate_revision_id
            or self.target_candidate_revision_hash != stage_input.target_candidate_revision_hash
        ):
            raise ValueError("Candidate repair Patch does not match its frozen target")
        allowed = {target.candidate_key: target for target in stage_input.allowed_targets}
        seen: set[tuple[str, str]] = set()
        for operation in self.operations:
            target = allowed.get(operation.target_candidate_key)
            if (
                target is None
                or operation.base_fragment_hash != target.base_fragment_hash
                or operation.field_name not in target.allowed_fields
            ):
                raise ValueError("Candidate repair Patch escaped its frozen allowlist or fragment")
            if (
                _replacement_kind(operation.replacement)
                != BIBLE_REPAIR_FIELD_TYPES[operation.field_name]
            ):
                raise ValueError(
                    "Candidate repair Patch replacement type does not match its frozen field"
                )
            identity = (operation.target_candidate_key, operation.field_name)
            if identity in seen:
                raise ValueError("Candidate repair Patch contains a duplicate operation")
            seen.add(identity)
        _validate_review_issues(
            self.review_issues,
            {_evidence_key(value) for value in stage_input.target_issue.evidence},
            require_evidence=False,
        )


def _evidence_key(value: Evidence) -> tuple[int, int, str, str, int | None]:
    return (
        value.source_start,
        value.source_end,
        value.text_hash,
        value.exact_anchor,
        value.episode_number,
    )


def _validate_episode_candidate_keys(
    fragments: list[StructureFragment],
    claims: list[ClaimCandidate],
    issues: list[CandidateIssue],
) -> None:
    fragment_keys = [value.temporary_key for value in fragments]
    claim_keys = [value.claim_key for value in claims]
    issue_keys = [value.issue_key for value in issues]
    if len(fragment_keys) != len(set(fragment_keys)):
        raise ValueError("Episode candidate fragment keys must be unique")
    if len(claim_keys) != len(set(claim_keys)):
        raise ValueError("Episode candidate claim keys must be unique")
    if len(issue_keys) != len(set(issue_keys)):
        raise ValueError("Episode candidate issue keys must be unique")
    if fragments != sorted(
        fragments,
        key=lambda value: (value.source_start, value.source_end, value.temporary_key),
    ):
        raise ValueError("Episode candidate fragments must be source ordered")


def _validate_episode_candidate_content(
    fragments: list[StructureFragment],
    claims: list[ClaimCandidate],
    issues: list[CandidateIssue],
    stage_input: Any,
    *,
    source_start: int,
    source_end: int,
    context_start: int | None = None,
    context_text: str | None = None,
) -> None:
    known_identities = {value.entity_key for value in stage_input.known_identities}
    known_states = {
        value.state_key for identity in stage_input.known_identities for value in identity.states
    }
    fragment_keys = {value.temporary_key for value in fragments}
    scene_keys = {value.temporary_key for value in fragments if value.kind == "scene"}
    for fragment in fragments:
        if fragment.source_start < source_start or fragment.source_end > source_end:
            raise ValueError("Episode candidate fragment escaped its frozen source range")
        attributes = fragment.attributes
        referenced_identities = {
            value
            for value in (
                attributes.speaker_key,
                attributes.location_key,
                attributes.occurrence_entity_key,
                *attributes.participant_keys,
            )
            if value is not None
        }
        if not referenced_identities.issubset(known_identities):
            raise ValueError("Episode candidate references an unknown known identity")
        if attributes.state_key is not None and attributes.state_key not in known_states:
            raise ValueError("Episode candidate references an unknown known identity state")
        if attributes.scene_key is not None and attributes.scene_key not in scene_keys:
            raise ValueError("Episode candidate references an unknown scene fragment")
        _validate_episode_evidence(
            fragment.evidence,
            source_start,
            source_end,
            stage_input.episode_position,
            context_start=context_start,
            context_text=context_text,
        )
    for claim in claims:
        if not set(claim.participant_keys).issubset(known_identities):
            raise ValueError("Episode candidate Claim references an unknown known identity")
        if not set(claim.anchor_keys).issubset(fragment_keys | known_identities):
            raise ValueError("Episode candidate Claim references an unknown anchor")
        _validate_episode_evidence(
            claim.evidence,
            source_start,
            source_end,
            stage_input.episode_position,
            context_start=context_start,
            context_text=context_text,
        )
    for issue in issues:
        _validate_episode_evidence(
            issue.evidence,
            source_start,
            source_end,
            stage_input.episode_position,
            context_start=context_start,
            context_text=context_text,
        )


def _validate_episode_evidence(
    values: list[Evidence],
    source_start: int,
    source_end: int,
    episode_position: int,
    *,
    context_start: int | None,
    context_text: str | None,
) -> None:
    for value in values:
        if (
            value.source_start < source_start
            or value.source_end > source_end
            or value.episode_number != episode_position
            or hashlib.sha256(value.exact_anchor.encode("utf-8")).hexdigest() != value.text_hash
        ):
            raise ValueError("Episode candidate Evidence escaped or drifted from its source")
        if context_start is not None and context_text is not None:
            relative_start = value.source_start - context_start
            relative_end = value.source_end - context_start
            if (
                relative_start < 0
                or relative_end > len(context_text)
                or context_text[relative_start:relative_end] != value.exact_anchor
            ):
                raise ValueError("Episode candidate Evidence does not match its frozen context")


def _replacement_kind(value: RepairReplacement) -> str:
    if value.text is not None:
        return "text"
    if value.integer is not None:
        return "integer"
    if value.flag is not None:
        return "flag"
    if value.strings is not None:
        return "strings"
    return ""


def _candidate_evidence_keys(
    candidate: StoryReconciliationCandidate,
) -> set[tuple[int, int, str, str, int | None]]:
    values: list[Evidence] = []
    for entity in candidate.canonical_entities:
        values.extend(entity.evidence)
        for state in entity.states:
            values.extend(state.evidence)
    for entry in candidate.canonical_world_entries:
        values.extend(entry.evidence)
    for claim in candidate.merged_claims:
        values.extend(claim.evidence)
    for arc in candidate.merged_arcs:
        values.extend(arc.evidence)
    for issue in (*candidate.conflicts, *candidate.review_issues):
        values.extend(issue.evidence)
    return {_evidence_key(value) for value in values}


def _validate_review_issues(
    issues: list[CandidateIssue],
    allowed_evidence: set[tuple[int, int, str, str, int | None]],
    *,
    require_evidence: bool,
) -> None:
    issue_keys: set[str] = set()
    for issue in issues:
        if issue.code in BIBLE_DETERMINISTIC_GATE_CODES:
            raise ValueError("model Review Issue cannot use a deterministic Gate code")
        if issue.issue_key in issue_keys:
            raise ValueError("StoryGraph review issue keys must be unique")
        issue_keys.add(issue.issue_key)
        if require_evidence and not issue.evidence:
            raise ValueError("StoryGraph Review Issue requires Evidence")
        if any(_evidence_key(value) not in allowed_evidence for value in issue.evidence):
            raise ValueError(
                "StoryGraph Review Issue Evidence is outside the frozen Candidate Revision"
            )
