"""Application service that executes registered Harness capabilities."""

from __future__ import annotations

from pydantic import BaseModel

from app.harness.scene_analysis_schemas import (
    SceneAnalysisInvocation,
)
from app.harness.schemas import StoryGraphStageInvocation
from app.modules.storygraph.harness import StoryGraphHarness
from app.modules.storygraph.scene_analysis_harness import SceneAnalysisHarness
from app.modules.text_storyboard.harness import TextHarness, TextResult, TextTask
from app.skills.runtime import SkillRuntime


class HarnessService:
    """Resolve Skills once through the application catalog before execution."""

    def __init__(self, skill_runtime: SkillRuntime) -> None:
        self.skill_runtime = skill_runtime

    async def storygraph(self, invocation: StoryGraphStageInvocation) -> tuple[BaseModel, str]:
        harness = StoryGraphHarness(invocation, skill_catalog=self.skill_runtime.catalog)
        try:
            value = await harness.execute()
            return value, harness.model_name
        finally:
            await harness.aclose()

    async def scene_analysis(self, invocation: SceneAnalysisInvocation) -> tuple[BaseModel, str]:
        harness = SceneAnalysisHarness(invocation, skill_catalog=self.skill_runtime.catalog)
        try:
            value = await harness.execute()
            return value, harness.model_name
        finally:
            await harness.aclose()

    async def text_storyboard(self, task: TextTask) -> TextResult:
        harness = TextHarness(skill_catalog=self.skill_runtime.catalog)
        return await harness.execute(task)
