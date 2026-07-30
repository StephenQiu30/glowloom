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
