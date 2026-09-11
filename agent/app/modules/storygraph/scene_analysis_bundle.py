from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from app.modules.storygraph.bundle import SKILL_BUNDLE_HASH, BundleInvalid, StoryGraphBundle
from app.modules.storygraph.scene_analysis_registry import scene_analysis_stage_spec

SCENE_ANALYSIS_SKILL_BUNDLE_HASH = SKILL_BUNDLE_HASH


@dataclass(frozen=True)
class SceneAnalysisBundleManifest:
    definition_version: str = "storygraph-scene-analysis"
    prompt_version: str = "build-storygraph-scene-analysis"
    skill_bundle_version: str = "build-storygraph-scene-analysis"
    skill_bundle_hash: str = SCENE_ANALYSIS_SKILL_BUNDLE_HASH
    model_capability: str = "structured_text"
    max_model_calls: int = 1
    max_execution_seconds: int = 120
    max_output_bytes: int = 131072
    allowed_tools: tuple[str, ...] = ()


class SceneAnalysisBundle:
    _STAGE_RESOURCE_PATHS = (
        "SKILL.md",
        "references/entity-reconciliation.md",
        "references/interaction-continuity.md",
        "references/production-entities.md",
        "references/scene-occurrences.md",
        "references/scene-facts.md",
        "references/script-spans.md",
        "references/structure-identity-review.md",
    )

    def __init__(self, repository_root: Path | None = None) -> None:
        root = repository_root or Path(__file__).resolve().parents[4]
        self.repository_root = root
        self.root = root / "agent" / "skills" / "build-storygraph"
        self.manifest = SceneAnalysisBundleManifest()

    def compute_hash(self) -> str:
        self._verify_root()
        return StoryGraphBundle(self.repository_root).compute_hash()

    def verify_installed_bundle(self) -> str:
        computed = self.compute_hash()
        if computed != self.manifest.skill_bundle_hash:
            raise BundleInvalid("Scene Analysis bundle hash does not match its release")
        return computed

    def loaded_paths(self, stage: str, profile: str) -> tuple[str, ...]:
        spec = scene_analysis_stage_spec(stage, profile)
        return ("SKILL.md", *(f"references/{name}" for name in spec.references))

    def guidance(self, stage: str, profile: str) -> str:
        self._verify_root()
        sections: list[str] = []
        for relative_path in self.loaded_paths(stage, profile):
            if relative_path not in self._STAGE_RESOURCE_PATHS:
                raise BundleInvalid("Scene Analysis stage requested an undeclared reference")
            path = self.root / relative_path
            if path.is_symlink():
                raise BundleInvalid("Scene Analysis stage reference is a symlink")
            try:
                sections.append(f"## {relative_path}\n{path.read_text(encoding='utf-8')}")
            except (OSError, UnicodeDecodeError) as error:
                raise BundleInvalid("Scene Analysis stage reference is unavailable") from error
        return "\n\n".join(sections)

    def _verify_root(self) -> None:
        root_chain = (
            self.repository_root,
            self.repository_root / "agent",
            self.repository_root / "agent" / "skills",
            self.root,
        )
        if any(path.is_symlink() for path in root_chain):
            raise BundleInvalid("Scene Analysis bundle root contains a symlink")
        try:
            repository_root = self.repository_root.resolve(strict=True)
            bundle_root = self.root.resolve(strict=True)
        except OSError as error:
            raise BundleInvalid("Scene Analysis bundle root is invalid") from error
        if (
            not repository_root.is_dir()
            or not bundle_root.is_dir()
            or not bundle_root.is_relative_to(repository_root)
        ):
            raise BundleInvalid("Scene Analysis bundle root is invalid")
