from __future__ import annotations

from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field


class ProductionBibleInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    bible_id: UUID
    task_id: UUID
    workspace_id: UUID
    project_id: UUID
    document_revision_id: UUID
    normalized_text: str = Field(min_length=1)
    run_token: UUID


class Evidence(BaseModel):
    model_config = ConfigDict(extra="forbid")

    source_start: int = Field(ge=0)
    source_end: int = Field(gt=0)
    text_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    exact_anchor: str = Field(min_length=1)
    episode_number: int | None = Field(ge=1)


class Observation(BaseModel):
    model_config = ConfigDict(extra="forbid")

    observation_key: str
    kind: Literal["entity", "entity_state", "world_entry"]
    parent_entity_key: str | None
    proposed_key: str
    label: str
    facts: list[str]
    evidence_refs: list[str] = Field(min_length=1)
    episode_numbers: list[int]
    ambiguities: list[str]


class EvidenceResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    observations: list[Observation]


class AssetSpec(BaseModel):
    model_config = ConfigDict(extra="forbid")

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
    source_kind: Literal[
        "synthetic_recording", "human_recording", "voice_clone"
    ] | None
    language: str | None
    performance_traits: list[str]
    allowed_usage: list[str]


class EntityState(BaseModel):
    model_config = ConfigDict(extra="forbid")

    state_key: str
    label: str
    state_spec: AssetSpec
    episode_numbers: list[int]
    evidence: list[Evidence] = Field(min_length=1)
    ambiguities: list[str]


class Entity(BaseModel):
    model_config = ConfigDict(extra="forbid")

    entity_key: str
    kind: Literal["character", "location", "prop", "costume", "visual_style", "voice"]
    canonical_name: str
    normalized_name: str
    aliases: list[str]
    stable_spec: AssetSpec
    episode_numbers: list[int]
    evidence: list[Evidence] = Field(min_length=1)
    states: list[EntityState]
    ambiguities: list[str]


class WorldEntry(BaseModel):
    model_config = ConfigDict(extra="forbid")

    entry_key: str
    category: str
    title: str
    facts: list[str]
    rules: list[str]
    entity_keys: list[str]
    episode_numbers: list[int]
    evidence: list[Evidence] = Field(min_length=1)
    ambiguities: list[str]


class ReviewIssue(BaseModel):
    model_config = ConfigDict(extra="forbid")

    issue_key: str
    code: str
    severity: Literal["warning", "blocking"]
    scope: Literal["global", "entity", "entity_state", "world_entry"]
    subject_key: str | None
    summary: str
    repair_hint: str | None
    evidence: list[Evidence]


class ProductionBibleCandidate(BaseModel):
    model_config = ConfigDict(extra="forbid")

    entities: list[Entity]
    world_entries: list[WorldEntry]
    review_issues: list[ReviewIssue]


class ReviewResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    review_issues: list[ReviewIssue]
