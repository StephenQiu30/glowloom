from dataclasses import dataclass
from uuid import UUID


@dataclass(frozen=True, slots=True)
class EpisodeContentContext:
    episode_id: UUID
    workspace_id: UUID
    current_script_version_id: UUID | None
    revision: int
