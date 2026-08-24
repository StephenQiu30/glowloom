from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field

from app.modules.storyboards.schemas import ShotSpec


class FrozenContract(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)


class ExportBlocker(FrozenContract):
    code: str
    summary: str
    next_action: str
    shot_id: UUID | None = None
    dependency_id: UUID | None = None


class ExportUnit(FrozenContract):
    narrative_unit_id: UUID
    unit_version_id: UUID
    position: int = Field(ge=1)
    kind: Literal["scene_heading", "action", "dialogue", "narration"]
    exact_text: str
    text_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    required_for_coverage: bool
    coverage_status: Literal["covered", "approved_omitted", "uncovered"]


class ExportAsset(FrozenContract):
    asset_id: UUID
    asset_state_id: UUID
    asset_version_id: UUID
    kind: str
    name: str
    state_label: str
    state_revision: int = Field(ge=1)
    content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    readiness_hash: str = Field(pattern=r"^[0-9a-f]{64}$")


class ExportAssetReference(FrozenContract):
    slot_key: str
    role: str
    asset_id: UUID
    asset_state_id: UUID
    asset_version_id: UUID
    binding_source: Literal["manual", "ai"]
    subject_key: str | None


class ExportNarrativeReference(FrozenContract):
    reference_id: UUID
    narrative_unit_id: UUID
    unit_version_id: UUID
    channel: Literal["visual", "audio", "both"]
    role: str
    coverage_mode: Literal["full", "partial"]
    segment_start: int | None
    segment_end: int | None
    contribution: Literal["required", "supporting"]
    origin: Literal["ai", "human", "migrated"]


class ExportShot(FrozenContract):
    shot_id: UUID
    shot_spec_version_id: UUID
    position: int = Field(ge=1)
    title: str
    spec_version_no: int = Field(ge=1)
    spec_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    spec_input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    spec: ShotSpec
    prompt: str
    readiness_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    asset_references: tuple[ExportAssetReference, ...]
    narrative_references: tuple[ExportNarrativeReference, ...]


class ExportSnapshot(FrozenContract):
    schema_label: Literal["lanverse.storyboard.export.snapshot.1"] = (
        "lanverse.storyboard.export.snapshot.1"
    )
    workspace_id: UUID
    project_id: UUID
    episode_id: UUID
    script_version_id: UUID
    script_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    narrative_structure_id: UUID
    narrative_structure_revision: int = Field(ge=1)
    narrative_dependency_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    coverage_basis_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    coverage_evaluation_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    readiness_evaluation_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    units: tuple[ExportUnit, ...]
    assets: tuple[ExportAsset, ...]
    shots: tuple[ExportShot, ...]


class ExportFile(FrozenContract):
    path: str
    media_type: str
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    size_bytes: int = Field(ge=1)


class PackageMember(FrozenContract):
    path: str
    media_type: str
    content: bytes


class PackageResult(FrozenContract):
    content: bytes
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    size_bytes: int = Field(ge=1)
    files: tuple[ExportFile, ...]


class ExportPreparation(FrozenContract):
    status: Literal["ready", "blocked", "unavailable"]
    snapshot: ExportSnapshot | None
    input_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    blockers: tuple[ExportBlocker, ...]
