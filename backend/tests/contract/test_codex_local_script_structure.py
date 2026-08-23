import os

import pytest

from app.core.config import get_settings
from app.integrations.codex_local import CodexLocalScriptStructureExtractor

pytestmark = pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_CODEX_LOCAL_CONTRACT") != "1",
    reason="set LANVERSE_RUN_CODEX_LOCAL_CONTRACT=1 with a logged-in local Codex",
)


@pytest.mark.asyncio
async def test_real_codex_extracts_source_anchored_script_structure() -> None:
    settings = get_settings()
    script = "内景·旧泵站·夜\n红色警报灯闪烁。\n沈岚：快关闸！"
    extractor = CodexLocalScriptStructureExtractor(
        codex_cli_path=settings.codex_cli_path,
        model=settings.codex_model,
        max_concurrency=1,
    )
    try:
        result = await extractor.extract(script, episode_number=1)
    finally:
        await extractor.aclose()

    scenes = [candidate for candidate in result.candidates if candidate.proposal.kind == "scene"]
    dialogues = [
        candidate for candidate in result.candidates if candidate.proposal.kind == "dialogue"
    ]
    assert scenes
    assert scenes[0].source_range.start == 0
    assert scenes[0].source_range.end == len(script)
    assert dialogues
    dialogue_text = "沈岚：快关闸！"
    assert dialogues[0].source_range.start == script.index(dialogue_text)
    assert dialogues[0].source_range.end == script.index(dialogue_text) + len(dialogue_text)
