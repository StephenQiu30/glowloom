from collections.abc import AsyncIterator

import pytest

from app.integrations.document_probe import Utf8DocumentProbe
from app.modules.media import MediaProbeError


async def _content(*chunks: bytes) -> AsyncIterator[bytes]:
    for chunk in chunks:
        yield chunk


@pytest.mark.asyncio
async def test_document_probe_accepts_strict_utf8_txt_and_markdown() -> None:
    probe = Utf8DocumentProbe(max_bytes=400_000, max_codepoints=100_000)

    txt = await probe.probe(
        _content("第一集\n正文🧭".encode()),
        kind="document",
        mime_type="text/plain",
    )
    markdown = await probe.probe(
        _content(b"# Episode 1\n", "正文".encode()),
        kind="document",
        mime_type="text/markdown",
    )

    assert txt.codec == "utf-8"
    assert txt.container == "text/plain"
    assert markdown.codec == "utf-8"
    assert markdown.container == "text/markdown"


@pytest.mark.asyncio
@pytest.mark.parametrize(
    ("content", "mime_type", "error_code"),
    [
        (b"\xff\xfeA\x00", "text/plain", "document_not_utf8"),
        (b"\xef\xbb\xbftext", "text/plain", "utf8_bom_not_allowed"),
        (b" \r\n ", "text/plain", "empty_document"),
        (b"text", "application/pdf", "unsupported_document_mime"),
    ],
)
async def test_document_probe_rejects_unsafe_or_unsupported_input(
    content: bytes,
    mime_type: str,
    error_code: str,
) -> None:
    probe = Utf8DocumentProbe(max_bytes=400_000, max_codepoints=100_000)

    with pytest.raises(MediaProbeError) as captured:
        await probe.probe(
            _content(content),
            kind="document",
            mime_type=mime_type,
        )

    assert captured.value.code == error_code


@pytest.mark.asyncio
async def test_document_probe_stops_at_the_bounded_byte_limit() -> None:
    probe = Utf8DocumentProbe(max_bytes=4, max_codepoints=100_000)

    with pytest.raises(MediaProbeError) as captured:
        await probe.probe(
            _content(b"1234", b"5"),
            kind="document",
            mime_type="text/plain",
        )

    assert captured.value.code == "document_too_large"
