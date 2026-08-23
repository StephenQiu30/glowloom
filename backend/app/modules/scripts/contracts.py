from dataclasses import dataclass
from typing import Literal, Protocol
from uuid import UUID

ScriptProductionStatus = Literal[
    "not_started",
    "published",
    "extracting",
    "extraction_blocked",
    "review_required",
    "confirmation_required",
    "set_current_required",
    "confirmed",
    "unavailable",
]


class ScriptVersionImpactReader(Protocol):
    async def __call__(
        self,
        *,
        episode_id: UUID,
        current_script_version_id: UUID,
    ) -> list[UUID]: ...


@dataclass(frozen=True, slots=True)
class NarrativeImpactSnapshot:
    impact_id: UUID
    previous_dependency_hash: str | None
    current_dependency_hash: str
    invalidated_scopes: tuple[str, ...]


@dataclass(frozen=True, slots=True)
class NarrativeDependencySnapshot:
    episode_id: UUID
    script_version_id: UUID
    structure_id: UUID
    structure_revision: int
    dependency_hash: str


@dataclass(frozen=True, slots=True)
class ScriptVersionSnapshot:
    workspace_id: UUID
    episode_id: UUID
    version_id: UUID
    version_no: int
    status: Literal["draft", "published"]
    content_hash: str


@dataclass(frozen=True, slots=True)
class StoryboardNarrativeUnit:
    narrative_unit_id: UUID
    unit_version_id: UUID
    position: int
    source_start: int
    source_end: int
    kind: Literal["scene_heading", "action", "dialogue", "narration"]
    exact_text: str
    text_hash: str
    required_for_coverage: bool
    source_scene_id: UUID | None
    source_dialogue_id: UUID | None


@dataclass(frozen=True, slots=True)
class StoryboardNarrativeSnapshot:
    workspace_id: UUID
    episode_id: UUID
    script_version_id: UUID
    structure_id: UUID
    structure_revision: int
    dependency_hash: str
    units: tuple[StoryboardNarrativeUnit, ...]


@dataclass(frozen=True, slots=True)
class NarrativeUnitVersionReference:
    workspace_id: UUID
    project_id: UUID
    episode_id: UUID
    script_version_id: UUID
    narrative_unit_id: UUID
    narrative_unit_version_id: UUID
    current_script_version_id: UUID | None
    current_unit_version_id: UUID | None
    text_hash: str

    @property
    def is_current(self) -> bool:
        return (
            self.script_version_id == self.current_script_version_id
            and self.narrative_unit_version_id == self.current_unit_version_id
        )


class NarrativeImpactRecorder(Protocol):
    async def __call__(
        self,
        *,
        workspace_id: UUID,
        episode_id: UUID,
        episode_revision: int,
        previous_script_version_id: UUID | None,
        current_script_version_id: UUID,
        affected_shot_ids: list[UUID],
        actor_id: UUID,
    ) -> NarrativeImpactSnapshot: ...


@dataclass(frozen=True, slots=True)
class ScriptProductionSummary:
    status: ScriptProductionStatus
    current_version_id: UUID | None
    extraction_batch_id: UUID | None = None
    pending_required_candidates: int = 0
    task_next_action: str | None = None


@dataclass(frozen=True, slots=True)
class ScriptExtractionInput:
    batch_id: UUID
    task_id: UUID
    workspace_id: UUID
    script_version_id: UUID
    episode_number: int
    body: str


@dataclass(frozen=True, slots=True)
class ConfirmedStructureReference:
    workspace_id: UUID
    episode_id: UUID
    script_version_id: UUID
    scene_id: UUID
    dialogue_ids: tuple[UUID, ...]


@dataclass(frozen=True, slots=True)
class ConfirmedStructureQuery:
    script_version_id: UUID
    scene_id: UUID
    dialogue_ids: tuple[UUID, ...]


@dataclass(frozen=True, slots=True)
class EpisodeConfirmedStructureQuery:
    episode_id: UUID
    structure: ConfirmedStructureQuery


@dataclass(frozen=True, slots=True)
class ConfirmedShotCandidateReference:
    candidate_id: UUID
    workspace_id: UUID
    episode_id: UUID
    script_version_id: UUID
    scene_id: UUID
    title: str
    purpose: str
