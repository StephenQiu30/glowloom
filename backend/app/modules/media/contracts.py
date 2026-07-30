from collections.abc import AsyncIterator
from dataclasses import dataclass
from typing import Protocol


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
