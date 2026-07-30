import os
from collections.abc import AsyncIterator

import pytest

from app.integrations.ffprobe import FfprobeMediaProbe
from tests.support.media_fixtures import ONE_PIXEL_PNG


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_FFPROBE_CONTRACT") != "1",
    reason="set LANVERSE_RUN_FFPROBE_CONTRACT=1 with ffprobe 8.1.2 installed",
)
@pytest.mark.asyncio
async def test_ffprobe_inspects_real_image_bytes() -> None:
    async def content() -> AsyncIterator[bytes]:
        yield ONE_PIXEL_PNG

    result = await FfprobeMediaProbe(timeout_seconds=10).probe(
        content(), kind="image", mime_type="image/png"
    )

    assert result.width == 1
    assert result.height == 1
    assert result.codec == "png"
    assert result.container == "png_pipe"
