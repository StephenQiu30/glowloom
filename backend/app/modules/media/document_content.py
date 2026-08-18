import io
import xml.etree.ElementTree as ET
import zipfile
from collections.abc import AsyncIterator

from app.modules.media.contracts import MediaProbeError

DOCUMENT_MIME_TYPES = frozenset(
    {
        "text/plain",
        "text/markdown",
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    }
)
DOCX_MIME_TYPE = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
MAX_DOCX_UNCOMPRESSED_BYTES = 256 * 1024 * 1024
_DOCX_NAMESPACE = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
_DOCX_TEXT = f"{{{_DOCX_NAMESPACE}}}t"
_DOCX_TAB = f"{{{_DOCX_NAMESPACE}}}tab"
_DOCX_BREAKS = frozenset(
    {
        f"{{{_DOCX_NAMESPACE}}}br",
        f"{{{_DOCX_NAMESPACE}}}cr",
    }
)


def _read_docx_text(
    body: bytes,
    *,
    max_bytes: int,
    max_codepoints: int | None,
) -> str:
    try:
        with zipfile.ZipFile(io.BytesIO(body)) as archive:
            infos = archive.infolist()
            max_uncompressed_bytes = min(
                max_bytes * 16,
                MAX_DOCX_UNCOMPRESSED_BYTES,
            )
            if sum(info.file_size for info in infos) > max_uncompressed_bytes:
                raise MediaProbeError(
                    "document_too_large", "DOCX uncompressed content exceeds its byte limit"
                )
            try:
                document_xml = archive.read("word/document.xml")
            except KeyError as error:
                raise MediaProbeError(
                    "invalid_probe_output", "DOCX document.xml is missing"
                ) from error
    except MediaProbeError:
        raise
    except (OSError, ValueError, zipfile.BadZipFile) as error:
        raise MediaProbeError("invalid_probe_output", "DOCX archive is invalid") from error

    try:
        root = ET.fromstring(document_xml)
    except ET.ParseError as error:
        raise MediaProbeError("invalid_probe_output", "DOCX document.xml is invalid") from error

    paragraphs: list[str] = []
    for paragraph in root.iter(f"{{{_DOCX_NAMESPACE}}}p"):
        parts: list[str] = []
        for element in paragraph.iter():
            if element.tag == _DOCX_TEXT:
                parts.append(element.text or "")
            elif element.tag == _DOCX_TAB:
                parts.append("\t")
            elif element.tag in _DOCX_BREAKS:
                parts.append("\n")
        paragraphs.append("".join(parts))
    text = "\n".join(paragraphs)
    if max_codepoints is not None and len(text) > max_codepoints:
        raise MediaProbeError("document_too_large", "Document exceeds its code-point limit")
    if not text.strip():
        raise MediaProbeError("empty_document", "Document must contain text")
    return text


async def read_strict_utf8_document(
    content: AsyncIterator[bytes],
    *,
    mime_type: str,
    max_bytes: int,
    max_codepoints: int | None = None,
) -> tuple[str, str]:
    normalized_mime = mime_type.split(";", 1)[0].strip().lower()
    if normalized_mime not in DOCUMENT_MIME_TYPES:
        raise MediaProbeError(
            "unsupported_document_mime",
            "Document must be UTF-8 text/plain, text/markdown or DOCX",
        )
    body = bytearray()
    async for chunk in content:
        if len(body) + len(chunk) > max_bytes:
            raise MediaProbeError("document_too_large", "Document exceeds its byte limit")
        body.extend(chunk)
    raw_body = bytes(body)
    if normalized_mime == DOCX_MIME_TYPE:
        return _read_docx_text(
            raw_body,
            max_bytes=max_bytes,
            max_codepoints=max_codepoints,
        ), normalized_mime
    try:
        text = raw_body.decode("utf-8", errors="strict")
    except UnicodeDecodeError as error:
        raise MediaProbeError("document_not_utf8", "Document is not valid UTF-8") from error
    if text.startswith("\ufeff"):
        raise MediaProbeError("utf8_bom_not_allowed", "UTF-8 BOM must be removed from the document")
    if max_codepoints is not None and len(text) > max_codepoints:
        raise MediaProbeError("document_too_large", "Document exceeds its code-point limit")
    if not text.strip():
        raise MediaProbeError("empty_document", "Document must contain text")
    return text, normalized_mime
