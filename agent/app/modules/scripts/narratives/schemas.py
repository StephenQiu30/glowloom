from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)


class SourceRange(BaseModel):
    start: int
    end: int


class NarrativeUnitResponse(BaseModel):
    id: UUID
    unit_id: UUID
    kind: Literal["scene_heading", "action", "dialogue", "narration"]
    position: int
    version_no: int
    source_range: SourceRange
    exact_text: str
    text_hash: str
    prefix_text: str
    suffix_text: str
    required_for_coverage: bool
    source_scene_id: UUID | None
    source_dialogue_id: UUID | None
    origin: Literal["deterministic", "manual"]
    created_at: datetime


class NarrativeStructureResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    episode_id: UUID
    script_version_id: UUID
    input_hash: str
    parser_version: str
    structure_hash: str
    dependency_hash: str
    revision: int
    units: list[NarrativeUnitResponse]
    created_at: datetime
    updated_at: datetime


class NarrativeUnitRevisionItem(CommandModel):
    unit_id: UUID
    kind: Literal["scene_heading", "action", "dialogue", "narration"]
    source_start: int = Field(ge=0)
    source_end: int = Field(ge=1)
    required_for_coverage: bool


class NarrativeStructureRevisionRequest(CommandModel):
    expected_revision: int = Field(ge=1)
    expected_current_script_version_id: UUID
    idempotency_key: str = Field(min_length=1, max_length=200)
    units: list[NarrativeUnitRevisionItem] = Field(min_length=1, max_length=500)

    @model_validator(mode="after")
    def validate_unit_ids(self) -> "NarrativeStructureRevisionRequest":
        unit_ids = [unit.unit_id for unit in self.units]
        if len(set(unit_ids)) != len(unit_ids):
            raise ValueError("unit IDs must be unique")
        return self


class NarrativeImpactResponse(BaseModel):
    id: UUID
    episode_id: UUID
    sequence: int
    trigger: Literal["current_changed", "structure_corrected"]
    episode_revision: int
    previous_script_version_id: UUID | None
    current_script_version_id: UUID
    previous_structure_hash: str | None
    current_structure_hash: str
    previous_dependency_hash: str | None
    current_dependency_hash: str
    previous_unit_count: int
    current_unit_count: int
    affected_shot_ids: list[UUID]
    invalidated_scopes: list[Literal["shot_readiness", "coverage", "export"]]
    impact_hash: str
    created_at: datetime


class NarrativeRevisionResponse(BaseModel):
    structure: NarrativeStructureResponse
    impact: NarrativeImpactResponse


class NarrativeDependencyResponse(BaseModel):
    episode_id: UUID
    current_script_version_id: UUID
    current_structure_id: UUID
    current_structure_revision: int
    current_dependency_hash: str
    evaluated_hash: str | None
    status: Literal["fresh", "stale"]
