from __future__ import annotations

import hashlib
from dataclasses import dataclass
from pathlib import Path

from app.modules.storygraph.skill_registry import RegistryError, stage_spec

SKILL_BUNDLE_HASH = "4cf64c94b7d181945da678721db36c4bc45921a9c833164bdea46cb7af149c42"


class BundleInvalid(ValueError):
    pass


@dataclass(frozen=True)
class BundleManifest:
    definition_key: str = "storygraph_stage"
    definition_version: str = "storygraph-stage-harness-v1"
    prompt_version: str = "build-storygraph-prompt-v1"
    skill_bundle_version: str = "build-storygraph-v1"
    skill_bundle_hash: str = SKILL_BUNDLE_HASH
    output_schema_version: str = "storygraph-candidate-schema-v1"
    model_capability: str = "structured_text"
    codex_runtime_contract: str = "codex-cli-ephemeral-read-only-v1"
    allowed_tools: tuple[str, ...] = ()
    max_model_calls: int = 2
    max_execution_seconds: int = 600


class StoryGraphBundle:
    _ALLOWED_PATHS = (
        "SKILL.md",
        "references/continuity-review.md",
        "references/entity-reconciliation.md",
        "references/episode-segmentation.md",
        "references/scene-structure.md",
        "references/shot-detail.md",
        "references/source-evidence.md",
        "references/story-analysis.md",
        "references/storyboard-table.md",
        "references/visual-identity.md",
    )

    def __init__(self, repository_root: Path | None = None) -> None:
        root = repository_root or Path(__file__).resolve().parents[4]
        self.root = root / "agent" / "skills" / "build-storygraph"
        self.manifest = BundleManifest()

    @classmethod
    def allowed_paths(cls) -> tuple[str, ...]:
        return cls._ALLOWED_PATHS

    def compute_hash(self) -> str:
        if self.root.is_symlink() or not self.root.is_dir():
            raise BundleInvalid("StoryGraph bundle root is invalid")
        actual: set[str] = set()
        for path in self.root.rglob("*"):
            if path.is_symlink():
                raise BundleInvalid("StoryGraph bundle contains a symlink")
            if path.is_file():
                actual.add(path.relative_to(self.root).as_posix())
        if actual != set(self._ALLOWED_PATHS):
            raise BundleInvalid("StoryGraph bundle file set is invalid")

        digest = hashlib.sha256()
        for relative_path in self._ALLOWED_PATHS:
            path = self.root / relative_path
            try:
                content = path.read_bytes()
                content.decode("utf-8")
            except (OSError, UnicodeDecodeError) as error:
                raise BundleInvalid("StoryGraph bundle contains invalid UTF-8") from error
            digest.update(relative_path.encode("utf-8"))
            digest.update(b"\0")
            digest.update(len(content).to_bytes(8, "big"))
            digest.update(content)
        return digest.hexdigest()

    def verify_installed_bundle(self) -> str:
        computed = self.compute_hash()
        if computed != self.manifest.skill_bundle_hash:
            raise BundleInvalid("StoryGraph bundle hash does not match the runtime manifest")
        return computed

    def loaded_paths(
        self, stage: str, stage_input: dict[str, object] | None = None
    ) -> tuple[str, ...]:
        spec = stage_spec(stage)
        references = list(spec.references)
        if stage == "review_storygraph":
            reviewed_stage = (stage_input or {}).get("reviewed_stage")
            if not isinstance(reviewed_stage, str) or reviewed_stage in {
                "review_storygraph",
                "repair_candidate",
            }:
                raise BundleInvalid("StoryGraph review target stage is invalid")
            try:
                reviewed_spec = stage_spec(reviewed_stage)
            except RegistryError as error:
                raise BundleInvalid("StoryGraph review target stage is invalid") from error
            for reference in reviewed_spec.references:
                if reference not in references:
                    references.append(reference)
        return ("SKILL.md", *(f"references/{name}" for name in references))

    def guidance(self, stage: str, stage_input: dict[str, object] | None = None) -> str:
        sections: list[str] = []
        for relative_path in self.loaded_paths(stage, stage_input):
            if relative_path not in self._ALLOWED_PATHS:
                raise BundleInvalid("StoryGraph stage requested an undeclared reference")
            path = self.root / relative_path
            if path.is_symlink():
                raise BundleInvalid("StoryGraph stage reference is a symlink")
            try:
                sections.append(f"## {relative_path}\n{path.read_text(encoding='utf-8')}")
            except (OSError, UnicodeDecodeError) as error:
                raise BundleInvalid("StoryGraph stage reference is unavailable") from error
        return "\n\n".join(sections)
