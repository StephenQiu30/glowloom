from __future__ import annotations

from typing import Any, Literal, cast
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator


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
    boundaries: list[EpisodeBoundary]
    review_issues: list[CandidateIssue]


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
    summary: str = Field(min_length=1)
    evidence: list[Evidence] = Field(min_length=1)
    attributes: StructureAttributes


class EpisodeAnalysisCandidate(StrictModel):
    fragments: list[StructureFragment]
    claims: list[ClaimCandidate]
    review_issues: list[CandidateIssue]


class EpisodeReconciliationCandidate(StrictModel):
    ordered_fragments: list[StructureFragment]
    conflicts: list[CandidateIssue]
    review_issues: list[CandidateIssue]


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
    deterministic_blockers: list[str]
    review_issues: list[CandidateIssue]


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
    target_temporary_key: str = Field(min_length=1)
    base_fragment_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    field_name: str = Field(min_length=1)
    replacement: RepairReplacement


class CandidateRepairPatch(StrictModel):
    target_candidate_revision_id: UUID
    target_candidate_revision_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    operations: list[RepairOperation] = Field(min_length=1)
    review_issues: list[CandidateIssue]
