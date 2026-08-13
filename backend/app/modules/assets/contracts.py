from dataclasses import dataclass
from typing import Literal, Protocol
from uuid import UUID


@dataclass(frozen=True, slots=True)
class AssetVersionReference:
    id: UUID
    workspace_id: UUID
    project_id: UUID
    asset_id: UUID
    asset_state_id: UUID
    kind: str
    asset_status: str
    asset_state_status: str


@dataclass(frozen=True, slots=True)
class AssetVersionReadinessReference:
    id: UUID
    asset_id: UUID
    asset_state_id: UUID
    kind: str
    asset_status: str
    asset_state_status: str
    status: Literal["draft", "ready", "blocked", "unavailable"]
    blocker_codes: tuple[str, ...]
    media_version_ids: tuple[UUID, ...]
    consent_ids: tuple[UUID, ...]
    evaluation_hash: str


@dataclass(frozen=True, slots=True)
class AssetCandidateCommand:
    workspace_id: UUID
    project_id: UUID
    candidate_id: UUID
    decision_key: str
    actor_id: UUID
    action: str
    kind: str
    name: str
    description: str
    target_asset_id: UUID | None = None


@dataclass(frozen=True, slots=True)
class AssetCandidateResult:
    asset_id: UUID
    asset_version_id: UUID | None


@dataclass(frozen=True, slots=True)
class AssetOccurrenceNarrativeSnapshot:
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


class AssetOccurrenceNarrativeReader(Protocol):
    async def __call__(
        self,
        *,
        workspace_id: UUID,
        unit_version_ids: list[UUID],
    ) -> dict[UUID, AssetOccurrenceNarrativeSnapshot]: ...


@dataclass(frozen=True, slots=True)
class ProjectAssetSummary:
    status: Literal["not_started", "draft", "blocked", "ready", "unavailable"]
    total: int
    versioned: int
    ready: int
    draft: int
    blocked: int
    ready_kinds: tuple[str, ...]
    required_kinds: tuple[str, ...] = ("character", "location", "voice")


@dataclass(frozen=True, slots=True)
class ProjectAssetReferenceSummary:
    asset_count: int
    version_count: int


class AssetCandidateDecisionCountReader(Protocol):
    async def __call__(
        self,
        *,
        workspace_id: UUID,
        asset_ids: list[UUID],
    ) -> dict[UUID, int]: ...
