from dataclasses import dataclass
from typing import Literal, Protocol
from uuid import UUID

from app.modules.scripts.planning.schemas import EpisodePlanningProviderResult

EPISODE_PLANNER_VERSION = "episode-plan-anchor-v1"
EPISODE_PLANNER_PROMPT_VERSION = "episode-plan-prompt-v2"
EPISODE_PLANNER_SCHEMA_VERSION = "episode-plan-schema-v1"


class EpisodePlanner(Protocol):
    async def plan(
        self,
        normalized_text: str,
        *,
        target_duration_ms: int,
        maximum_episode_count: int,
    ) -> EpisodePlanningProviderResult: ...


class EpisodePlanningProviderError(Exception):
    def __init__(
        self,
        *,
        outcome: Literal["failed", "unknown"],
        code: str,
        summary: str,
        retryable: bool,
        next_action: str,
    ) -> None:
        super().__init__(summary)
        self.outcome = outcome
        self.code = code
        self.summary = summary
        self.retryable = retryable
        self.next_action = next_action


@dataclass(frozen=True, slots=True)
class EpisodePlanningInput:
    plan_id: UUID
    task_id: UUID
    workspace_id: UUID
    document_revision_id: UUID
    input_hash: str
    normalized_text: str
    target_duration_ms: int
    maximum_episode_count: int
