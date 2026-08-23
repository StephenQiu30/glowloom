import os

import pytest

from app.core.config import get_settings
from app.integrations.deepseek import DeepSeekScriptAdapter
from app.modules.scripts.adaptations import adaptation_duration_bounds

pytestmark = pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_DEEPSEEK_ADAPTATION_CONTRACT") != "1",
    reason=("set LANVERSE_RUN_DEEPSEEK_ADAPTATION_CONTRACT=1 with a configured DEEPSEEK_API_KEY"),
)


@pytest.mark.asyncio
async def test_real_deepseek_adaptation_respects_structured_duration_gate() -> None:
    settings = get_settings()
    assert settings.deepseek_api_key is not None
    original = "\n".join(
        [
            "第一集",
            "场景1：雾港控制室，夜",
            "林澜：封锁港口，先救孩子。",
            "警报突然响起，屏幕显示闸门失控。",
        ]
    )
    target_duration_ms = 45_000

    result = await DeepSeekScriptAdapter(settings.deepseek_api_key).adapt(
        original,
        target_duration_ms=target_duration_ms,
        core_plot_points=["林澜封锁港口", "先救孩子", "结尾闸门失控"],
        pacing="fast",
        colloquial_dialogue=True,
    )

    duration_lower_ms, duration_upper_ms = adaptation_duration_bounds(target_duration_ms)
    assert duration_lower_ms <= result.estimated_duration_ms <= duration_upper_ms
    assert result.adapted_script_text.strip()
    assert result.adapted_script_text != original
    assert result.change_summary.strip()
    assert all(
        keyword in result.adapted_script_text for keyword in ("林澜", "港口", "孩子", "闸门")
    )
