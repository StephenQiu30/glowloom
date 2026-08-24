from datetime import datetime
from typing import Annotated, Any, Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, TypeAdapter, model_validator

AssetKind = Literal[
    "character",
    "location",
    "prop",
    "costume",
    "visual_style",
    "voice",
]
AssetStatus = Literal["active", "archived"]
AssetAvailability = Literal["enabled", "disabled"]
ReadinessStatus = Literal["draft", "ready", "blocked"]
AssetStateStatus = Literal["active", "disabled"]
OccurrenceDecision = Literal["link", "unlink"]
OccurrenceFreshness = Literal["current", "stale"]


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class CharacterSpec(CommandModel):
    kind: Literal["character"]
    identity: str = Field(default="", max_length=300)
    appearance: str = Field(default="", max_length=4000)
    age_impression: str = Field(default="", max_length=200)
    temperament: list[str] = Field(default_factory=list, max_length=20)
    goals: list[str] = Field(default_factory=list, max_length=50)
    relationships: list[str] = Field(default_factory=list, max_length=100)
    arc_summary: str = Field(default="", max_length=4000)
    voice_profile: str = Field(default="", max_length=1000)


class LocationSpec(CommandModel):
    kind: Literal["location"]
    spatial_description: str = Field(default="", max_length=4000)
    time_weather: str = Field(default="", max_length=500)
    visual_elements: list[str] = Field(default_factory=list, max_length=50)
    lighting: str = Field(default="", max_length=2000)


class PropSpec(CommandModel):
    kind: Literal["prop"]
    appearance: str = Field(default="", max_length=4000)
    material: str = Field(default="", max_length=1000)
    usage_context: str = Field(default="", max_length=2000)
    holder_character_id: UUID | None = None


class CostumeSpec(CommandModel):
    kind: Literal["costume"]
    appearance: str = Field(default="", max_length=4000)
    material: str = Field(default="", max_length=1000)
    usage_context: str = Field(default="", max_length=2000)
    wearer_character_id: UUID | None = None


class StyleSpec(CommandModel):
    kind: Literal["visual_style"]
    visual_language: str = Field(default="", max_length=4000)
    palette: str = Field(default="", max_length=1000)
    lighting_language: str = Field(default="", max_length=2000)
    negative_constraints: list[str] = Field(default_factory=list, max_length=50)


class VoiceSpec(CommandModel):
    kind: Literal["voice"]
    source_kind: Literal["synthetic_recording", "human_recording", "voice_clone"] | None = None
    language: str = Field(default="", max_length=35)
    performance_traits: list[str] = Field(default_factory=list, max_length=30)
    allowed_usage: list[str] = Field(default_factory=list, max_length=30)


AssetSpec = Annotated[
    CharacterSpec | LocationSpec | PropSpec | CostumeSpec | StyleSpec | VoiceSpec,
    Field(discriminator="kind"),
]

_SPEC_ADAPTER: TypeAdapter[AssetSpec] = TypeAdapter(AssetSpec)
_SPEC_MODELS = {
    "character": CharacterSpec,
    "location": LocationSpec,
    "prop": PropSpec,
    "costume": CostumeSpec,
    "visual_style": StyleSpec,
    "voice": VoiceSpec,
}


def parse_asset_spec(kind: str, payload: dict[str, object]) -> AssetSpec:
    model = _SPEC_MODELS.get(kind)
    if model is None:
        return _SPEC_ADAPTER.validate_python(payload)
    return model.model_validate(payload)


class AssetCreateRequest(CommandModel):
    kind: AssetKind
    name: str = Field(min_length=1, max_length=200)
    aliases: list[str] = Field(default_factory=list, max_length=30)
    tags: list[str] = Field(default_factory=list, max_length=30)


class AssetUpdateRequest(CommandModel):
    expected_revision: int = Field(ge=1)
    aliases: list[str] | None = Field(default=None, max_length=30)
    tags: list[str] | None = Field(default=None, max_length=30)


class AssetStatusRequest(CommandModel):
    expected_revision: int = Field(ge=1)


class AssetEnableRequest(AssetStatusRequest):
    idempotency_key: str = Field(min_length=1, max_length=200)


class AssetResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    project_id: UUID
    kind: AssetKind
    name: str
    aliases: list[str]
    tags: list[str]
    status: AssetStatus
    availability: AssetAvailability
    name_revision: int
    revision: int
    created_at: datetime
    updated_at: datetime
    warnings: list[Literal["duplicate_name"]] = []


class PaginatedAssets(BaseModel):
    items: list[AssetResponse]
    total: int
    limit: int
    offset: int


class AssetStateCreateRequest(CommandModel):
    state_key: str = Field(pattern=r"^[a-z0-9][a-z0-9_]{0,79}$")
    label: str = Field(min_length=1, max_length=120)
    description: str = Field(default="", max_length=4000)
    expected_asset_revision: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)


class AssetStateResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    asset_id: UUID
    state_key: str
    label: str
    description: str
    status: AssetStateStatus
    current_version_id: UUID | None
    revision: int
    created_by: UUID
    created_at: datetime
    updated_at: datetime


class AssetStateCreateResponse(BaseModel):
    asset: AssetResponse
    state: AssetStateResponse


class AssetStateUpdateRequest(CommandModel):
    expected_revision: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)
    label: str | None = Field(default=None, min_length=1, max_length=120)
    description: str | None = Field(default=None, max_length=4000)


class AssetStateEnableRequest(AssetStatusRequest):
    idempotency_key: str = Field(min_length=1, max_length=200)


class PaginatedAssetStates(BaseModel):
    items: list[AssetStateResponse]
    total: int


class AssetMediaReferenceRequest(CommandModel):
    media_version_id: UUID
    purpose: Literal[
        "portrait",
        "full_body",
        "expression",
        "turnaround",
        "environment",
        "object",
        "outfit",
        "style_reference",
        "voice_sample",
    ]
    position: int = Field(ge=1, le=100)


class AssetMediaReferenceResponse(BaseModel):
    media_version_id: UUID
    purpose: str
    position: int


class AssetVersionCreateRequest(CommandModel):
    spec: AssetSpec
    prompt_description: str = Field(default="", max_length=8000)
    media_references: list[AssetMediaReferenceRequest] = Field(default=[], max_length=100)
    source_type: Literal["manual", "script_extraction_candidate"] = "manual"
    source_id: UUID | None = None
    expected_revision: int = Field(ge=1)
    expected_current_version_id: UUID | None
    set_as_current: bool = True

    @model_validator(mode="after")
    def validate_source(self) -> "AssetVersionCreateRequest":
        if self.source_type == "script_extraction_candidate" and self.source_id is None:
            raise ValueError("script extraction candidate source requires source_id")
        if self.source_type == "manual" and self.source_id is not None:
            raise ValueError("manual source does not accept source_id")
        keys = [(item.purpose, item.position) for item in self.media_references]
        if len(set(keys)) != len(keys):
            raise ValueError("media reference purpose and position must be unique")
        return self


class AssetVersionResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    asset_id: UUID
    asset_state_id: UUID
    version_no: int
    schema_version: int
    spec: AssetSpec
    prompt_description: str
    source_type: Literal[
        "manual",
        "script_extraction_candidate",
        "production_bible_state",
    ]
    source_id: UUID | None
    content_hash: str
    media_references: list[AssetMediaReferenceResponse]
    created_by: UUID
    created_at: datetime


class PaginatedAssetVersions(BaseModel):
    items: list[AssetVersionResponse]
    total: int
    limit: int
    offset: int


class AssetStateCurrentPreflightRequest(CommandModel):
    version_id: UUID
    expected_current_version_id: UUID | None
    expected_revision: int = Field(ge=1)


class AssetStateCurrentRequest(AssetStateCurrentPreflightRequest):
    impact_hash: str = Field(min_length=64, max_length=64)
    idempotency_key: str = Field(min_length=1, max_length=200)


class AssetRenamePreflightRequest(CommandModel):
    new_name: str = Field(min_length=1, max_length=200)
    expected_revision: int = Field(ge=1)


class AssetRenameRequest(AssetRenamePreflightRequest):
    impact_hash: str = Field(min_length=64, max_length=64)
    idempotency_key: str = Field(min_length=1, max_length=200)


class AssetDisablePreflightRequest(CommandModel):
    expected_revision: int = Field(ge=1)


class AssetDisableRequest(AssetDisablePreflightRequest):
    impact_hash: str = Field(min_length=64, max_length=64)
    idempotency_key: str = Field(min_length=1, max_length=200)


class AssetImpactSummary(BaseModel):
    episode_count: int = Field(ge=0)
    shot_count: int = Field(ge=0)
    spec_version_count: int = Field(ge=0)
    prompt_snapshot_count: int = Field(ge=0)
    active_task_count: int = Field(ge=0)


class AssetEpisodeImpact(BaseModel):
    episode_id: UUID
    shot_count: int = Field(ge=0)
    prompt_snapshot_count: int = Field(ge=0)
    active_task_count: int = Field(ge=0)


class AssetShotImpact(BaseModel):
    shot_id: UUID
    shot_title: str
    episode_id: UUID
    spec_version_ids: list[UUID]
    current_spec_version_id: UUID | None
    slot_keys: list[str]


class AssetPromptImpact(BaseModel):
    generation_request_id: UUID
    episode_id: UUID
    shot_id: UUID
    shot_spec_version_id: UUID
    input_hash: str


class AssetTaskImpact(BaseModel):
    task_id: UUID
    generation_request_id: UUID
    status: Literal["queued", "running", "waiting_provider", "unknown"]
    revision: int = Field(ge=1)


class AssetImpactResponse(BaseModel):
    operation: Literal["rename", "disable_asset", "disable_state", "set_current"]
    asset_id: UUID
    state_id: UUID | None
    old_version_id: UUID | None
    new_version_id: UUID | None
    summary: AssetImpactSummary
    episodes: list[AssetEpisodeImpact]
    shots: list[AssetShotImpact]
    prompt_snapshots: list[AssetPromptImpact]
    active_tasks: list[AssetTaskImpact]
    impact_hash: str


class AssetRenameResponse(BaseModel):
    asset: AssetResponse
    impact: AssetImpactResponse


class AssetAvailabilityResponse(BaseModel):
    asset: AssetResponse
    impact: AssetImpactResponse


class AssetStateAvailabilityResponse(BaseModel):
    state: AssetStateResponse
    impact: AssetImpactResponse


class AssetStateCurrentResponse(BaseModel):
    state: AssetStateResponse
    impact: AssetImpactResponse


class AssetReadinessBlocker(BaseModel):
    code: str
    field_path: str | None = None
    dependency_type: str | None = None
    dependency_id: UUID | None = None
    summary: str
    next_action: str


class AssetReadinessDependencySnapshot(BaseModel):
    asset_version_id: UUID
    asset_state_id: UUID
    asset_state_revision: int
    media_version_ids: list[UUID]
    consent_ids: list[UUID]
    evaluated_at: datetime


class AssetReadinessResponse(BaseModel):
    status: ReadinessStatus
    blockers: list[AssetReadinessBlocker]
    warnings: list[str]
    next_actions: list[str]
    dependency_snapshot: AssetReadinessDependencySnapshot


class AssetVersionCreateResponse(BaseModel):
    state: AssetStateResponse
    version: AssetVersionResponse
    readiness: AssetReadinessResponse


class AssetOccurrenceRequest(CommandModel):
    decision: OccurrenceDecision
    narrative_unit_id: UUID
    narrative_unit_version_id: UUID
    expected_revision: int = Field(ge=1)
    idempotency_key: str = Field(min_length=1, max_length=200)


class AssetOccurrenceResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    asset_state_id: UUID
    episode_id: UUID
    narrative_unit_id: UUID
    narrative_unit_version_id: UUID
    sequence: int
    decision: OccurrenceDecision
    origin: Literal["manual", "script_candidate"]
    evidence_hash: str
    idempotency_key: str
    freshness: OccurrenceFreshness
    created_by: UUID
    created_at: datetime


class AssetOccurrenceDecisionResponse(BaseModel):
    state: AssetStateResponse
    decision: AssetOccurrenceResponse


class PaginatedAssetOccurrences(BaseModel):
    items: list[AssetOccurrenceResponse]
    total: int


class AssetStateReadinessSnapshot(BaseModel):
    asset_state_id: UUID
    asset_state_revision: int
    current_version_id: UUID | None
    occurrence_decision_ids: list[UUID]
    media_version_ids: list[UUID]
    consent_ids: list[UUID]
    evaluated_at: datetime


class AssetStateReadinessResponse(BaseModel):
    status: Literal["draft", "ready", "blocked", "unavailable"]
    blockers: list[AssetReadinessBlocker]
    warnings: list[str]
    next_actions: list[str]
    dependency_snapshot: AssetStateReadinessSnapshot


class AssetBibleState(BaseModel):
    state: AssetStateResponse
    current_version: AssetVersionResponse | None
    occurrences: list[AssetOccurrenceResponse]
    readiness: AssetStateReadinessResponse


class AssetBibleAsset(BaseModel):
    asset: AssetResponse
    states: list[AssetBibleState]


class AssetBibleSummary(BaseModel):
    asset_count: int
    state_count: int
    ready: int
    draft: int
    blocked: int
    unavailable: int


class AssetBibleResponse(BaseModel):
    items: list[AssetBibleAsset]
    summary: AssetBibleSummary


class AssetDeleteBlocker(BaseModel):
    code: str
    summary: str
    version_count: int = Field(ge=0)
    decision_count: int = Field(ge=0)
    related_version_count: int = Field(ge=0)


class AssetDeletePreflightResponse(BaseModel):
    allowed: bool
    blockers: list[AssetDeleteBlocker]


class AssetDeleteResponse(BaseModel):
    deleted: Literal[True] = True


def spec_to_json(spec: AssetSpec) -> dict[str, Any]:
    return spec.model_dump(mode="json")
