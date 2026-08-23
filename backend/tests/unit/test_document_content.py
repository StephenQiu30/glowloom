import io
import zipfile

import pytest

from app.modules.media.contracts import MediaProbeError
from app.modules.media.document_content import read_strict_utf8_document


def _docx_bytes() -> bytes:
    xml = """<?xml version="1.0" encoding="UTF-8"?>
    <w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
      <w:body>
        <w:p><w:r><w:t>第一场</w:t></w:r></w:p>
        <w:p><w:r><w:t>角色甲：</w:t><w:tab/><w:t>开始。</w:t></w:r></w:p>
      </w:body>
    </w:document>
    """
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, "w") as archive:
        archive.writestr("word/document.xml", xml)
    return buffer.getvalue()


async def _content(body: bytes):
    yield body


@pytest.mark.asyncio
async def test_reads_docx_paragraphs_as_script_text() -> None:
    text, mime_type = await read_strict_utf8_document(
        _content(_docx_bytes()),
        mime_type="application/vnd.openxmlformats-officedocument.wordprocessingml.document",
        max_bytes=100_000,
        max_codepoints=10_000,
    )

    assert mime_type == ("application/vnd.openxmlformats-officedocument.wordprocessingml.document")
    assert text == "第一场\n角色甲：\t开始。"


@pytest.mark.asyncio
async def test_rejects_invalid_docx_without_returning_partial_text() -> None:
    with pytest.raises(MediaProbeError, match="DOCX archive is invalid") as error:
        await read_strict_utf8_document(
            _content(b"not a docx"),
            mime_type="application/vnd.openxmlformats-officedocument.wordprocessingml.document",
            max_bytes=100_000,
            max_codepoints=10_000,
        )

    assert error.value.code == "invalid_probe_output"
