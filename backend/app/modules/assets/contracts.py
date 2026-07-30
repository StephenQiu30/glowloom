from dataclasses import dataclass
from uuid import UUID


@dataclass(frozen=True, slots=True)
class AssetVersionReference:
    id: UUID
    workspace_id: UUID
    project_id: UUID
    asset_id: UUID
    kind: str
    asset_status: str


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
