from collections.abc import AsyncIterator

from app.modules.media import MediaProbeError, MediaProbePort, MediaProbeResult
from app.modules.media.document_content import read_strict_utf8_document


class Utf8DocumentProbe:
    def __init__(self, *, max_bytes: int, max_codepoints: int) -> None:
        self._max_bytes = max_bytes
        self._max_codepoints = max_codepoints

    async def probe(
        self,
        content: AsyncIterator[bytes],
        *,
        kind: str,
        mime_type: str,
    ) -> MediaProbeResult:
        if kind != "document":
            raise MediaProbeError(
                "unsupported_document_mime",
                "Document must be UTF-8 text/plain or text/markdown",
            )
        _, normalized_mime = await read_strict_utf8_document(
            content,
            mime_type=mime_type,
            max_bytes=self._max_bytes,
            max_codepoints=self._max_codepoints,
        )
        return MediaProbeResult(codec="utf-8", container=normalized_mime)


class RoutingMediaProbe:
    def __init__(
        self,
        *,
        media_probe: MediaProbePort,
        document_probe: MediaProbePort,
    ) -> None:
        self._media_probe = media_probe
        self._document_probe = document_probe

    async def probe(
        self,
        content: AsyncIterator[bytes],
        *,
        kind: str,
        mime_type: str,
    ) -> MediaProbeResult:
        target = self._document_probe if kind == "document" else self._media_probe
        return await target.probe(content, kind=kind, mime_type=mime_type)
