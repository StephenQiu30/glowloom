from datetime import datetime
from hashlib import sha256
from typing import Literal, Self, TypeAlias
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

BibleStatus: TypeAlias = Literal[
    "queued",
    "running",
    "needs_review",
    "confirmed",
    "failed",
    "unknown",
    "superseded",
    "cancelled",
]
BibleEntityKind: TypeAlias = Literal[
    "character",
    "location",
    "prop",
    "costume",
    "visual_style",
    "voice",
]
EpisodeNumber: TypeAlias = int
_KEY_PATTERN = r"^[a-z0-9][a-z0-9_.:-]{0,99}$"
_STATE_KEY_PATTERN = r"^[a-z0-9][a-z0-9_]{0,79}$"
_HASH_PATTERN = r"^[0-9a-f]{64}$"


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)


def _validate_ordered_unique_numbers(values: list[int], field_name: str) -> None:
    if any(value < 1 for value in values):
        raise ValueError(f"{field_name} must contain positive episode numbers")
    if values != sorted(set(values)):
        raise ValueError(f"{field_name} must contain sorted unique episode numbers")


def _validate_unique_text(values: list[str], field_name: str) -> None:
    if any(not value.strip() for value in values):
        raise ValueError(f"{field_name} cannot contain blank values")
    normalized = [" ".join(value.casefold().split()) for value in values]
    if len(set(normalized)) != len(normalized):
        raise ValueError(f"{field_name} must be unique after normalization")


class BibleEvidence(StrictModel):
    source_start: int = Field(ge=0)
    source_end: int = Field(ge=1)
    text_hash: str = Field(pattern=_HASH_PATTERN)
    exact_anchor: str = Field(min_length=1)
    episode_number: EpisodeNumber | None = None

    @model_validator(mode="after")
    def validate_anchor(self) -> Self:
        if self.source_end <= self.source_start:
            raise ValueError("source_end must be greater than source_start")
        if self.episode_number is not None and self.episode_number < 1:
            raise ValueError("episode_number must be positive")
        if len(self.exact_anchor) != self.source_end - self.source_start:
            raise ValueError("exact_anchor length must match the source range")
        if sha256(self.exact_anchor.encode()).hexdigest() != self.text_hash:
            raise ValueError("text_hash must match exact_anchor")
        return self


class BibleReviewIssue(StrictModel):
    issue_key: str = Field(pattern=_KEY_PATTERN)
    code: str = Field(min_length=1, max_length=120)
    severity: Literal["warning", "blocking"]
    scope: Literal["global", "entity", "entity_state", "world_entry"]
    subject_key: str | None = Field(default=None, min_length=1, max_length=100)
    summary: str = Field(min_length=1, max_length=2000)
    repair_hint: str | None = Field(default=None, max_length=2000)
    evidence: list[BibleEvidence] = Field(default_factory=list[BibleEvidence], max_length=100)

    @model_validator(mode="after")
    def validate_scope(self) -> Self:
        if self.scope == "global" and self.subject_key is not None:
            raise ValueError("global review issues cannot name a subject")
        if self.scope != "global" and self.subject_key is None:
            raise ValueError("scoped review issues require subject_key")
        return self


class BibleEvidenceObservation(StrictModel):
    observation_key: str = Field(pattern=_KEY_PATTERN)
    kind: Literal["entity", "entity_state", "world_entry"]
    subject_key: str = Field(pattern=_KEY_PATTERN)
    parent_entity_key: str | None = Field(default=None, pattern=_KEY_PATTERN)
    claim: str = Field(min_length=1, max_length=4000)
    evidence: list[BibleEvidence] = Field(min_length=1, max_length=100)
    ambiguities: list[str] = Field(default_factory=list, max_length=50)

    @model_validator(mode="after")
    def validate_parent(self) -> Self:
        if self.kind == "entity_state" and self.parent_entity_key is None:
            raise ValueError("entity_state observations require parent_entity_key")
        if self.kind != "entity_state" and self.parent_entity_key is not None:
            raise ValueError("only entity_state observations accept parent_entity_key")
        _validate_unique_text(self.ambiguities, "ambiguities")
        return self


class BibleEvidenceChunkResult(StrictModel):
    chunk_key: str = Field(pattern=_KEY_PATTERN)
    source_start: int = Field(ge=0)
    source_end: int = Field(ge=1)
    observations: list[BibleEvidenceObservation] = Field(
        default_factory=list[BibleEvidenceObservation], max_length=2000
    )

    @model_validator(mode="after")
    def validate_chunk_scope(self) -> Self:
        if self.source_end <= self.source_start:
            raise ValueError("source_end must be greater than source_start")
        observation_keys = [item.observation_key for item in self.observations]
        if len(set(observation_keys)) != len(observation_keys):
            raise ValueError("observation keys must be unique within a chunk")
        if any(
            evidence.source_start < self.source_start or evidence.source_end > self.source_end
            for observation in self.observations
            for evidence in observation.evidence
        ):
            raise ValueError("observation evidence must stay within the chunk")
        return self


class BibleAssetSpecCandidate(StrictModel):
    kind: BibleEntityKind | None = None
    identity: str | None = Field(default=None, max_length=300)
    appearance: str | None = Field(default=None, max_length=4000)
    age_impression: str | None = Field(default=None, max_length=200)
    temperament: list[str] | None = Field(default=None, max_length=20)
    goals: list[str] | None = Field(default=None, max_length=50)
    relationships: list[str] | None = Field(default=None, max_length=100)
    arc_summary: str | None = Field(default=None, max_length=4000)
    voice_profile: str | None = Field(default=None, max_length=1000)
    spatial_description: str | None = Field(default=None, max_length=4000)
    time_weather: str | None = Field(default=None, max_length=500)
    visual_elements: list[str] | None = Field(default=None, max_length=50)
    lighting: str | None = Field(default=None, max_length=2000)
    material: str | None = Field(default=None, max_length=1000)
    usage_context: str | None = Field(default=None, max_length=2000)
    visual_language: str | None = Field(default=None, max_length=4000)
    palette: str | None = Field(default=None, max_length=1000)
    lighting_language: str | None = Field(default=None, max_length=2000)
    negative_constraints: list[str] | None = Field(default=None, max_length=50)
    source_kind: Literal["synthetic_recording", "human_recording", "voice_clone"] | None = None
    language: str | None = Field(default=None, max_length=35)
    performance_traits: list[str] | None = Field(default=None, max_length=30)
    allowed_usage: list[str] | None = Field(default=None, max_length=30)

    def validate_for_kind(self, kind: BibleEntityKind) -> None:
        if self.kind is not None and self.kind != kind:
            raise ValueError("Bible asset spec kind must match its entity kind")
        allowed_fields = {
            "character": {
                "identity",
                "appearance",
                "age_impression",
                "temperament",
                "goals",
                "relationships",
                "arc_summary",
                "voice_profile",
            },
            "location": {
                "spatial_description",
                "time_weather",
                "visual_elements",
                "lighting",
            },
            "prop": {"appearance", "material", "usage_context"},
            "costume": {"appearance", "material", "usage_context"},
            "visual_style": {
                "visual_language",
                "palette",
                "lighting_language",
                "negative_constraints",
            },
            "voice": {
                "source_kind",
                "language",
                "performance_traits",
                "allowed_usage",
            },
        }[kind]
        populated_fields = {
            field_name
            for field_name, value in self.model_dump(exclude={"kind"}).items()
            if value is not None
        }
        if unsupported := populated_fields - allowed_fields:
            raise ValueError(
                "Bible asset spec contains fields for another entity kind: "
                + ", ".join(sorted(unsupported))
            )

    def to_payload(self) -> dict[str, object]:
        return self.model_dump(mode="json", exclude_none=True)


class BibleEntityStateCandidate(StrictModel):
    state_key: str = Field(pattern=_STATE_KEY_PATTERN)
    label: str = Field(min_length=1, max_length=120)
    state_spec: BibleAssetSpecCandidate = Field(default_factory=BibleAssetSpecCandidate)
    episode_numbers: list[EpisodeNumber] = Field(default_factory=list[int], max_length=1000)
    evidence: list[BibleEvidence] = Field(min_length=1, max_length=2000)
    ambiguities: list[str] = Field(default_factory=list, max_length=50)

    @model_validator(mode="after")
    def validate_lists(self) -> Self:
        _validate_ordered_unique_numbers(self.episode_numbers, "episode_numbers")
        _validate_unique_text(self.ambiguities, "ambiguities")
        return self


class BibleEntityCandidate(StrictModel):
    entity_key: str = Field(pattern=_KEY_PATTERN)
    kind: BibleEntityKind
    canonical_name: str = Field(min_length=1, max_length=200)
    normalized_name: str = Field(min_length=1, max_length=200)
    aliases: list[str] = Field(default_factory=list, max_length=100)
    stable_spec: BibleAssetSpecCandidate = Field(default_factory=BibleAssetSpecCandidate)
    episode_numbers: list[EpisodeNumber] = Field(default_factory=list[int], max_length=1000)
    evidence: list[BibleEvidence] = Field(min_length=1, max_length=5000)
    states: list[BibleEntityStateCandidate] = Field(
        default_factory=list[BibleEntityStateCandidate], max_length=500
    )
    ambiguities: list[str] = Field(default_factory=list, max_length=100)

    @model_validator(mode="after")
    def validate_identity(self) -> Self:
        expected_name = " ".join(self.canonical_name.strip().casefold().split())
        if self.normalized_name != expected_name:
            raise ValueError("normalized_name must be the normalized canonical_name")
        _validate_unique_text(self.aliases, "aliases")
        _validate_ordered_unique_numbers(self.episode_numbers, "episode_numbers")
        _validate_unique_text(self.ambiguities, "ambiguities")
        normalized_aliases = {" ".join(value.casefold().split()) for value in self.aliases}
        if self.normalized_name in normalized_aliases:
            raise ValueError("aliases cannot repeat canonical_name")
        self.stable_spec.validate_for_kind(self.kind)
        for state in self.states:
            state.state_spec.validate_for_kind(self.kind)
        state_keys = [state.state_key for state in self.states]
        if len(set(state_keys)) != len(state_keys):
            raise ValueError("state keys must be unique within an entity")
        return self


class BibleWorldEntryCandidate(StrictModel):
    entry_key: str = Field(pattern=_KEY_PATTERN)
    category: str = Field(min_length=1, max_length=80)
    title: str = Field(min_length=1, max_length=200)
    facts: list[str] = Field(default_factory=list, max_length=500)
    rules: list[str] = Field(default_factory=list, max_length=500)
    entity_keys: list[str] = Field(default_factory=list, max_length=500)
    episode_numbers: list[EpisodeNumber] = Field(default_factory=list[int], max_length=1000)
    evidence: list[BibleEvidence] = Field(min_length=1, max_length=5000)
    ambiguities: list[str] = Field(default_factory=list, max_length=100)

    @model_validator(mode="after")
    def validate_content(self) -> Self:
        if not self.facts and not self.rules:
            raise ValueError("world entries require at least one fact or rule")
        _validate_unique_text(self.facts, "facts")
        _validate_unique_text(self.rules, "rules")
        _validate_unique_text(self.entity_keys, "entity_keys")
        _validate_ordered_unique_numbers(self.episode_numbers, "episode_numbers")
        _validate_unique_text(self.ambiguities, "ambiguities")
        return self


class ProductionBibleProviderResult(StrictModel):
    entities: list[BibleEntityCandidate] = Field(
        default_factory=list[BibleEntityCandidate], max_length=5000
    )
    world_entries: list[BibleWorldEntryCandidate] = Field(
        default_factory=list[BibleWorldEntryCandidate], max_length=5000
    )
    review_issues: list[BibleReviewIssue] = Field(
        default_factory=list[BibleReviewIssue], max_length=5000
    )

    @model_validator(mode="after")
    def validate_result_keys(self) -> Self:
        entity_keys = [entity.entity_key for entity in self.entities]
        if len(set(entity_keys)) != len(entity_keys):
            raise ValueError("entity keys must be unique")
        world_keys = [entry.entry_key for entry in self.world_entries]
        if len(set(world_keys)) != len(world_keys):
            raise ValueError("world entry keys must be unique")
        known_entities = set(entity_keys)
        unknown_references = {
            entity_key
            for entry in self.world_entries
            for entity_key in entry.entity_keys
            if entity_key not in known_entities
        }
        if unknown_references:
            raise ValueError("world entries reference unknown entity keys")
        return self


class ProductionBibleReviewResult(StrictModel):
    review_issues: list[BibleReviewIssue] = Field(
        default_factory=list[BibleReviewIssue], max_length=5000
    )


class ProductionBibleCreateRequest(StrictModel):
    idempotency_key: str = Field(min_length=1, max_length=200)


class ProductionBibleConfirmRequest(StrictModel):
    expected_revision: int = Field(ge=1)
    expected_result_hash: str = Field(pattern=_HASH_PATTERN)
    idempotency_key: str = Field(min_length=1, max_length=200)


class ProductionBibleResumeRequest(StrictModel):
    expected_revision: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)


class ProductionBibleEntityStateEpisodeNumbersCorrection(StrictModel):
    kind: Literal["entity_state_episode_numbers"]
    entity_key: str = Field(pattern=_KEY_PATTERN)
    state_key: str = Field(pattern=_STATE_KEY_PATTERN)
    episode_numbers: list[EpisodeNumber] = Field(default_factory=list[int], max_length=1000)

    @model_validator(mode="after")
    def validate_episode_numbers(self) -> Self:
        _validate_ordered_unique_numbers(self.episode_numbers, "episode_numbers")
        return self


class ProductionBibleReviewIssueResolutionRequest(StrictModel):
    expected_revision: int = Field(ge=1)
    expected_result_hash: str = Field(pattern=_HASH_PATTERN)
    idempotency_key: str = Field(min_length=1, max_length=200)
    issue_key: str = Field(pattern=_KEY_PATTERN)
    resolution_note: str = Field(min_length=1, max_length=2000)
    correction: ProductionBibleEntityStateEpisodeNumbersCorrection


class ProductionBibleEntityStateResponse(StrictModel):
    id: UUID
    entity_id: UUID
    state_key: str
    label: str
    state_spec: dict[str, object]
    episode_numbers: list[int]
    evidence: list[BibleEvidence]
    asset_state_id: UUID | None
    asset_version_id: UUID | None
    created_at: datetime
    updated_at: datetime


class ProductionBibleEntityResponse(StrictModel):
    id: UUID
    entity_key: str
    kind: BibleEntityKind
    canonical_name: str
    normalized_name: str
    aliases: list[str]
    stable_spec: dict[str, object]
    episode_numbers: list[int]
    evidence: list[BibleEvidence]
    asset_id: UUID | None
    states: list[ProductionBibleEntityStateResponse] = Field(
        default_factory=list[ProductionBibleEntityStateResponse]
    )
    created_at: datetime
    updated_at: datetime


class ProductionBibleWorldEntryResponse(StrictModel):
    id: UUID
    entry_key: str
    category: str
    title: str
    facts: list[str]
    rules: list[str]
    entity_keys: list[str]
    episode_numbers: list[int]
    evidence: list[BibleEvidence]
    created_at: datetime
    updated_at: datetime


class ProductionBibleResponse(StrictModel):
    id: UUID
    workspace_id: UUID
    project_id: UUID
    document_revision_id: UUID
    task_id: UUID | None
    status: BibleStatus
    input_hash: str = Field(pattern=_HASH_PATTERN)
    result_hash: str | None = Field(default=None, pattern=_HASH_PATTERN)
    engine_version: str
    model_name: str
    prompt_version: str
    schema_version: str
    harness_version: str
    checkpoint_stage: str | None
    checkpoint_revision: int = Field(ge=0)
    checkpoint_updated_at: datetime | None
    review_issues: list[BibleReviewIssue]
    revision: int = Field(ge=1)
    confirmed_at: datetime | None
    confirmed_by: UUID | None
    entities: list[ProductionBibleEntityResponse] = Field(
        default_factory=list[ProductionBibleEntityResponse]
    )
    world_entries: list[ProductionBibleWorldEntryResponse] = Field(
        default_factory=list[ProductionBibleWorldEntryResponse]
    )
    created_at: datetime
    updated_at: datetime
