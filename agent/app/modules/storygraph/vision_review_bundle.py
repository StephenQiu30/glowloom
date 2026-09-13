from __future__ import annotations

from dataclasses import replace
from pathlib import Path

from app.modules.storygraph.scene_analysis_bundle import SceneAnalysisBundle


class VisionReviewBundle(SceneAnalysisBundle):
    """Vision capability over the existing pinned StoryGraph resource bundle."""

    def __init__(self, repository_root: Path | None = None) -> None:
        super().__init__(repository_root)
        self.manifest = replace(
            self.manifest,
            definition_version="storygraph-vision-review",
            prompt_version="build-storygraph-vision-review",
            skill_bundle_version="build-storygraph",
            model_capability="vision",
        )
