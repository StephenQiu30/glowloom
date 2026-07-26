from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

from schemas.subtitles import SubtitleContentV1


class SubtitleVersionNotFound(LookupError):
    pass


@dataclass(frozen=True, slots=True)
class CreateSubtitlesCommand:
    episode_id: UUID
    idempotency_key: str


@dataclass(frozen=True, slots=True)
class SaveSubtitleCommand:
    version_id: UUID
    expected_resource_version: int
    content: SubtitleContentV1


@dataclass(frozen=True, slots=True)
class ConfirmSubtitleCommand:
    version_id: UUID
    expected_resource_version: int


@dataclass(frozen=True, slots=True)
class DeriveSubtitleDraftCommand:
    version_id: UUID
    idempotency_key: str
