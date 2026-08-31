from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from pathlib import Path

from app.modules.storygraph.bundle import BundleInvalid, StoryGraphBundle
from app.modules.storygraph.scene_analysis_registry import (
    SCENE_ANALYSIS_REGISTRY,
    scene_analysis_stage_spec,
)

SCENE_ANALYSIS_SKILL_BUNDLE_HASH = (
    "d096f3d38ff5383d685b2a510cea25985978e294a2a8c46841fa15320eee7b71"
)


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
    _BUNDLE_PATHS = StoryGraphBundle.known_paths()
    _STAGE_RESOURCE_PATHS = (
        "SKILL.md",
        "references/scene-facts.md",
        "references/script-spans.md",
    )

    def __init__(self, repository_root: Path | None = None) -> None:
        root = repository_root or Path(__file__).resolve().parents[4]
        self.repository_root = root
        self.root = root / "agent" / "skills" / "build-storygraph"
        self.manifest = SceneAnalysisBundleManifest()

    def compute_hash(self) -> str:
        self._verify_root()
        actual: set[str] = set()
        for path in self.root.rglob("*"):
            if path.is_symlink():
                raise BundleInvalid("StoryGraph bundle contains a symlink")
            if path.is_file():
                actual.add(path.relative_to(self.root).as_posix())
        if actual != set(StoryGraphBundle.known_paths()):
            raise BundleInvalid("StoryGraph bundle file set is invalid")

        digest = hashlib.sha256()
        digest.update(b"lanverse.storygraph.scene-analysis.bundle\0")
        manifest_identity = json.dumps(
            {
                "definition_version": self.manifest.definition_version,
                "prompt_version": self.manifest.prompt_version,
                "skill_bundle_version": self.manifest.skill_bundle_version,
                "model_capability": self.manifest.model_capability,
                "max_model_calls": self.manifest.max_model_calls,
                "max_execution_seconds": self.manifest.max_execution_seconds,
                "max_output_bytes": self.manifest.max_output_bytes,
            },
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("utf-8")
        digest.update(b"manifest-identity\0")
        digest.update(len(manifest_identity).to_bytes(8, "big"))
        digest.update(manifest_identity)
        for relative_path in sorted(self._BUNDLE_PATHS):
            path = self.root / relative_path
            try:
                content = path.read_bytes()
                content.decode("utf-8")
            except (OSError, UnicodeDecodeError) as error:
                raise BundleInvalid("Scene Analysis bundle contains invalid UTF-8") from error
            digest.update(relative_path.encode("utf-8"))
            digest.update(b"\0")
            digest.update(len(content).to_bytes(8, "big"))
            digest.update(content)
        for stage in sorted(SCENE_ANALYSIS_REGISTRY):
            schema = json.dumps(
                scene_analysis_stage_spec(stage).candidate_model.model_json_schema(),
                ensure_ascii=False,
                separators=(",", ":"),
                sort_keys=True,
            ).encode("utf-8")
            identity = f"output-schema:{stage}".encode()
            digest.update(identity)
            digest.update(b"\0")
            digest.update(len(schema).to_bytes(8, "big"))
            digest.update(schema)
        tool_policy = json.dumps(
            sorted(self.manifest.allowed_tools),
            ensure_ascii=False,
            separators=(",", ":"),
        ).encode("utf-8")
        digest.update(b"allowed-tools\0")
        digest.update(len(tool_policy).to_bytes(8, "big"))
        digest.update(tool_policy)
        return digest.hexdigest()

    def verify_installed_bundle(self) -> str:
        computed = self.compute_hash()
        if computed != self.manifest.skill_bundle_hash:
            raise BundleInvalid("Scene Analysis bundle hash does not match its release")
        return computed

    def loaded_paths(self, stage: str) -> tuple[str, ...]:
        spec = scene_analysis_stage_spec(stage)
        return ("SKILL.md", *(f"references/{name}" for name in spec.references))

    def guidance(self, stage: str) -> str:
        self._verify_root()
        sections: list[str] = []
        for relative_path in self.loaded_paths(stage):
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
