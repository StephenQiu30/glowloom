from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Literal, Protocol
from uuid import UUID


@dataclass(frozen=True, slots=True)
class MediaVersionReference:
    id: UUID
    workspace_id: UUID
    kind: str
    object_status: str
    probe_status: str
    has_active_location: bool


RenderedMediaSourceType = Literal[
    "asset_version",
    "narrative_unit_version",
    "script_version",
    "shot_spec_version",
    "storyboard_coverage",
    "storyboard_export_snapshot",
    "storyboard_readiness",
]


@dataclass(frozen=True, slots=True)
class RenderedMediaSource:
    source_type: RenderedMediaSourceType
    source_id: UUID
    source_hash: str
    position: int


@dataclass(frozen=True, slots=True)
class RenderedMediaCommand:
    workspace_id: UUID
    filename: str
    sha256: str
    size_bytes: int
    mime_type: str
    storage_profile: str
    bucket: str
    object_key: str
    created_by: UUID
    sources: tuple[RenderedMediaSource, ...]


@dataclass(frozen=True, slots=True)
class RenderedMediaResult:
    media_object_id: UUID
    media_version_id: UUID
    media_location_id: UUID


@dataclass(frozen=True, slots=True)
class MediaProbeResult:
    width: int | None = None
    height: int | None = None
    duration_ms: int | None = None
    codec: str | None = None
    container: str | None = None


class MediaProbeError(Exception):
    def __init__(self, code: str, summary: str) -> None:
        super().__init__(summary)
        self.code = code
        self.summary = summary


class MediaProbePort(Protocol):
    async def probe(
        self,
        content: AsyncIterator[bytes],
        *,
        kind: str,
        mime_type: str,
    ) -> MediaProbeResult: ...
