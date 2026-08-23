import os

import pytest

from app.core.config import get_settings
from app.integrations.codex_local import CodexLocalScriptAdapter
from app.modules.scripts.adaptations import adaptation_duration_bounds

pytestmark = pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_CODEX_LOCAL_CONTRACT") != "1",
    reason="set LANVERSE_RUN_CODEX_LOCAL_CONTRACT=1 with a logged-in local Codex",
)


@pytest.mark.asyncio
async def test_real_codex_adaptation_respects_structured_duration_gate() -> None:
    settings = get_settings()
    original = "\n".join(
        [
            "第一集",
            "场景1：雾港控制室，夜",
            "林澜：封锁港口，先救孩子。",
            "警报突然响起，屏幕显示闸门失控。",
        ]
    )
    target_duration_ms = 45_000

    adapter = CodexLocalScriptAdapter(
        codex_cli_path=settings.codex_cli_path,
        codex_model=settings.codex_model,
        max_concurrency=1,
    )
    try:
        result = await adapter.adapt(
            original,
            target_duration_ms=target_duration_ms,
            core_plot_points=["林澜封锁港口", "先救孩子", "结尾闸门失控"],
            pacing="fast",
            colloquial_dialogue=True,
        )
    finally:
        await adapter.aclose()

    duration_lower_ms, duration_upper_ms = adaptation_duration_bounds(target_duration_ms)
    assert duration_lower_ms <= result.estimated_duration_ms <= duration_upper_ms
    assert result.adapted_script_text.strip()
    assert result.adapted_script_text != original
    assert result.change_summary.strip()
    assert all(
        keyword in result.adapted_script_text for keyword in ("林澜", "港口", "孩子", "闸门")
    )
