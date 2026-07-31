from dataclasses import dataclass
from typing import Literal
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
class ConfirmedShotCandidateReference:
    candidate_id: UUID
    workspace_id: UUID
    episode_id: UUID
    script_version_id: UUID
    scene_id: UUID
    title: str
    purpose: str
