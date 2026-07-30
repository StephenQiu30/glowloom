import os

import pytest

from app.core.config import Settings
from app.integrations.deepseek import DeepSeekScriptStructureExtractor


@pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_DEEPSEEK_CONTRACT") != "1",
    reason="set LANVERSE_RUN_DEEPSEEK_CONTRACT=1 with DEEPSEEK_API_KEY configured",
)
@pytest.mark.asyncio
async def test_deepseek_extracts_typed_candidates_from_real_api() -> None:
    script_body = "第一场 室内 白天\n角色甲：我们开始吧。\n角色乙：好。"
    api_key = Settings().deepseek_api_key
    if api_key is None:
        pytest.fail("DEEPSEEK_API_KEY is required for the real provider contract")

    result = await DeepSeekScriptStructureExtractor(api_key).extract(script_body)

    assert result.candidates
    assert any(item.proposal.kind == "scene" for item in result.candidates)
    assert any(item.proposal.kind == "dialogue" for item in result.candidates)
    assert all(
        item.source_range.end <= len(script_body) for item in result.candidates
    )
