import os

import pytest

from app.core.config import get_settings
from app.integrations.deepseek import DeepSeekEpisodePlanner

pytestmark = pytest.mark.skipif(
    os.getenv("LANVERSE_RUN_DEEPSEEK_EPISODE_PLAN_CONTRACT") != "1",
    reason=("set LANVERSE_RUN_DEEPSEEK_EPISODE_PLAN_CONTRACT=1 with a configured DEEPSEEK_API_KEY"),
)


@pytest.mark.asyncio
async def test_real_deepseek_episode_plan_returns_verbatim_unique_anchors() -> None:
    settings = get_settings()
    assert settings.deepseek_api_key is not None
    text = "\n".join(
        [
            "场景一：海港控制室，夜",
            "警报突然响起。",
            "林澈发现三号闸门被远程锁死。",
            "同伴带回一份被删除的检修日志。",
            "场景二：旧仓库，雨",
            "日志显示事故发生前有人改过潮汐数据。",
            "林澈决定潜入灯塔机房。",
            "门外传来追兵脚步。",
            "场景三：灯塔机房，黎明",
            "林澈恢复备份并公开日志。",
            "闸门重新开启。",
            "远处又亮起第二组未知警报。",
        ]
    )

    result = await DeepSeekEpisodePlanner(settings.deepseek_api_key).plan(
        text,
        target_duration_ms=60_000,
        maximum_episode_count=10,
    )

    positions = [proposal.end_block_position for proposal in result.proposals]
    assert positions
    assert positions == sorted(set(positions))
    assert positions[-1] == len(text.splitlines())
    assert all(
        proposal.exact_end_anchor in text
        and text.count(proposal.exact_end_anchor) == 1
        and proposal.reason
        for proposal in result.proposals
    )
