from collections.abc import AsyncIterator

from app.modules.media.contracts import MediaProbeError

DOCUMENT_MIME_TYPES = frozenset({"text/plain", "text/markdown"})


async def read_strict_utf8_document(
    content: AsyncIterator[bytes],
    *,
    mime_type: str,
    max_bytes: int,
    max_codepoints: int,
) -> tuple[str, str]:
    normalized_mime = mime_type.split(";", 1)[0].strip().lower()
    if normalized_mime not in DOCUMENT_MIME_TYPES:
        raise MediaProbeError(
            "unsupported_document_mime",
            "Document must be UTF-8 text/plain or text/markdown",
        )
    body = bytearray()
    async for chunk in content:
        if len(body) + len(chunk) > max_bytes:
            raise MediaProbeError(
                "document_too_large", "Document exceeds its byte limit"
            )
        body.extend(chunk)
    try:
        text = bytes(body).decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise MediaProbeError(
            "document_not_utf8", "Document is not valid UTF-8"
        ) from error
    if text.startswith("\ufeff"):
        raise MediaProbeError(
            "utf8_bom_not_allowed", "UTF-8 BOM must be removed from the document"
        )
    if len(text) > max_codepoints:
        raise MediaProbeError(
            "document_too_large", "Document exceeds its code-point limit"
        )
    if not text.strip():
        raise MediaProbeError("empty_document", "Document must contain text")
    return text, normalized_mime
