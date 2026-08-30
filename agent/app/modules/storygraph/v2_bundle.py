from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from pathlib import Path

from app.modules.storygraph.bundle import BundleInvalid, StoryGraphBundle
from app.modules.storygraph.v2_registry import V2_REGISTRY, stage_spec_v2

STORYGRAPH_V2_SKILL_BUNDLE_HASH = "13f294e3fc9a241d07af80547a792de04d5357270622f3a2361bab84580e5de6"


@dataclass(frozen=True)
class V2BundleManifest:
    definition_version: str = "storygraph-scene-fact-v2"
    prompt_version: str = "build-storygraph-scene-fact-v1"
    skill_bundle_version: str = "build-storygraph-v3-scene-fact"
    skill_bundle_hash: str = STORYGRAPH_V2_SKILL_BUNDLE_HASH
    model_capability: str = "structured_text"
    max_model_calls: int = 1
    max_execution_seconds: int = 120
    max_output_bytes: int = 131072


class StoryGraphV2Bundle:
    _ALLOWED_PATHS = (
        "SKILL.md",
        "references/scene-facts.md",
        "references/script-spans.md",
    )

    def __init__(self, repository_root: Path | None = None) -> None:
        root = repository_root or Path(__file__).resolve().parents[4]
        self.root = root / "agent" / "skills" / "build-storygraph"
        self.manifest = V2BundleManifest()

    def compute_hash(self) -> str:
        if self.root.is_symlink() or not self.root.is_dir():
            raise BundleInvalid("StoryGraph v2 bundle root is invalid")
        actual: set[str] = set()
        for path in self.root.rglob("*"):
            if path.is_symlink():
                raise BundleInvalid("StoryGraph bundle contains a symlink")
            if path.is_file():
                actual.add(path.relative_to(self.root).as_posix())
        if actual != set(StoryGraphBundle.known_paths()):
            raise BundleInvalid("StoryGraph bundle file set is invalid")

        digest = hashlib.sha256()
        digest.update(b"lanverse.storygraph.bundle.v2\0")
        for relative_path in sorted(self._ALLOWED_PATHS):
            path = self.root / relative_path
            try:
                content = path.read_bytes()
                content.decode("utf-8")
            except (OSError, UnicodeDecodeError) as error:
                raise BundleInvalid("StoryGraph v2 bundle contains invalid UTF-8") from error
            digest.update(relative_path.encode("utf-8"))
            digest.update(b"\0")
            digest.update(len(content).to_bytes(8, "big"))
            digest.update(content)
        for stage in sorted(V2_REGISTRY):
            schema = json.dumps(
                stage_spec_v2(stage).candidate_model.model_json_schema(),
                ensure_ascii=False,
                separators=(",", ":"),
                sort_keys=True,
            ).encode("utf-8")
            identity = f"output-schema:{stage}".encode()
            digest.update(identity)
            digest.update(b"\0")
            digest.update(len(schema).to_bytes(8, "big"))
            digest.update(schema)
        tool_policy = b"[]"
        digest.update(b"allowed-tools\0")
        digest.update(len(tool_policy).to_bytes(8, "big"))
        digest.update(tool_policy)
        return digest.hexdigest()

    def verify_installed_bundle(self) -> str:
        computed = self.compute_hash()
        if computed != self.manifest.skill_bundle_hash:
            raise BundleInvalid("StoryGraph v2 bundle hash does not match its release")
        return computed

    def loaded_paths(self, stage: str) -> tuple[str, ...]:
        spec = stage_spec_v2(stage)
        return ("SKILL.md", *(f"references/{name}" for name in spec.references))

    def guidance(self, stage: str) -> str:
        sections: list[str] = []
        for relative_path in self.loaded_paths(stage):
            if relative_path not in self._ALLOWED_PATHS:
                raise BundleInvalid("StoryGraph v2 stage requested an undeclared reference")
            path = self.root / relative_path
            if path.is_symlink():
                raise BundleInvalid("StoryGraph v2 stage reference is a symlink")
            try:
                sections.append(f"## {relative_path}\n{path.read_text(encoding='utf-8')}")
            except (OSError, UnicodeDecodeError) as error:
                raise BundleInvalid("StoryGraph v2 stage reference is unavailable") from error
        return "\n\n".join(sections)
