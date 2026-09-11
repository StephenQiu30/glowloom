from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path

from app.modules.storygraph.bundle import SKILL_BUNDLE_HASH, BundleInvalid, StoryGraphBundle

VISUAL_FOUNDATION_SKILL_BUNDLE_HASH = SKILL_BUNDLE_HASH


@dataclass(frozen=True)
class VisualFoundationBundleManifest:
    definition_version: str = "storygraph-visual-foundation"
    prompt_version: str = "build-storygraph-visual-foundation"
    skill_bundle_version: str = "build-storygraph"
    skill_bundle_hash: str = VISUAL_FOUNDATION_SKILL_BUNDLE_HASH
    model_capability: str = "vision"
    max_model_calls: int = 1
    max_execution_seconds: int = 120
    max_output_bytes: int = 131072
    allowed_tools: tuple[str, ...] = ()


class VisualFoundationBundle:
    _LOADED_PATHS = ("SKILL.md", "references/visual-identity.md")

    def __init__(self, repository_root: Path | None = None) -> None:
        root = repository_root or Path(__file__).resolve().parents[4]
        self.root = root / "agent" / "skills" / "build-storygraph"
        self._bundle = StoryGraphBundle(root)
        self.manifest = VisualFoundationBundleManifest()

    def compute_hash(self) -> str:
        return self._bundle.compute_hash()

    def verify_installed_bundle(self) -> str:
        computed = self.compute_hash()
        if computed != self.manifest.skill_bundle_hash:
            raise BundleInvalid("Visual Foundation bundle hash does not match its release")
        return computed

    def loaded_paths(self) -> tuple[str, ...]:
        return self._LOADED_PATHS

    def guidance(self) -> str:
        self.verify_installed_bundle()
        sections: list[str] = []
        for relative_path in self._LOADED_PATHS:
            path = self.root / relative_path
            if path.is_symlink():
                raise BundleInvalid("Visual Foundation reference is a symlink")
            try:
                sections.append(f"## {relative_path}\n{path.read_text(encoding='utf-8')}")
            except (OSError, UnicodeDecodeError) as error:
                raise BundleInvalid("Visual Foundation reference is unavailable") from error
        return "\n\n".join(sections)
