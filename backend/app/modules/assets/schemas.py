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
ReadinessStatus = Literal["draft", "ready", "blocked"]


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class CharacterSpec(CommandModel):
    kind: Literal["character"]
    identity: str = Field(default="", max_length=300)
    appearance: str = Field(default="", max_length=4000)
    age_impression: str = Field(default="", max_length=200)
    temperament: list[str] = Field(default_factory=list, max_length=20)


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
    source_kind: Literal[
        "synthetic_recording", "human_recording", "voice_clone"
    ] | None = None
    language: str = Field(default="", max_length=35)
    performance_traits: list[str] = Field(default_factory=list, max_length=30)
    allowed_usage: list[str] = Field(default_factory=list, max_length=30)


AssetSpec = Annotated[
    CharacterSpec
    | LocationSpec
    | PropSpec
    | CostumeSpec
    | StyleSpec
    | VoiceSpec,
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
    name: str | None = Field(default=None, min_length=1, max_length=200)
    aliases: list[str] | None = Field(default=None, max_length=30)
    tags: list[str] | None = Field(default=None, max_length=30)


class AssetStateRequest(CommandModel):
    expected_revision: int = Field(ge=1)


class AssetResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    project_id: UUID
    kind: AssetKind
    name: str
    aliases: list[str]
    tags: list[str]
    status: AssetStatus
    current_version_id: UUID | None
    revision: int
    created_at: datetime
    updated_at: datetime
    warnings: list[Literal["duplicate_name"]] = []


class PaginatedAssets(BaseModel):
    items: list[AssetResponse]
    total: int
    limit: int
    offset: int


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
    source_type: Literal["manual", "candidate"] = "manual"
    source_id: UUID | None = None
    expected_current_version_id: UUID | None
    set_as_current: bool = True

    @model_validator(mode="after")
    def validate_source(self) -> "AssetVersionCreateRequest":
        if self.source_type == "candidate" and self.source_id is None:
            raise ValueError("candidate source requires source_id")
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
    version_no: int
    schema_version: int
    spec: AssetSpec
    prompt_description: str
    source_type: Literal["manual", "candidate"]
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


class AssetCurrentVersionRequest(CommandModel):
    version_id: UUID
    expected_current_version_id: UUID | None
    expected_revision: int = Field(ge=1)


class AssetReadinessBlocker(BaseModel):
    code: str
    field_path: str | None = None
    dependency_type: str | None = None
    dependency_id: UUID | None = None
    summary: str
    next_action: str


class AssetReadinessDependencySnapshot(BaseModel):
    asset_version_id: UUID
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
    asset: AssetResponse
    version: AssetVersionResponse
    readiness: AssetReadinessResponse


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
