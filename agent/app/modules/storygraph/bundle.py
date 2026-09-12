from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from pathlib import Path

from app.modules.storygraph.skill_registry import RegistryError, stage_spec

SKILL_BUNDLE_HASH = "c1b3da8229d3ff184ea8360b21e87d0fff36008d200943833d36e93e56cb2b42"


class BundleInvalid(ValueError):
    pass


@dataclass(frozen=True)
class BundleManifest:
    definition_key: str = "storygraph_stage"
    definition_version: str = "storygraph-stage-harness"
    prompt_version: str = "build-storygraph-prompt"
    skill_bundle_version: str = "build-storygraph"
    skill_bundle_hash: str = SKILL_BUNDLE_HASH
    output_schema_version: str = "storygraph-candidate-schema"
    model_capability: str = "structured_text"
    codex_runtime_contract: str = "codex-cli-ephemeral-read-only"
    allowed_tools: tuple[str, ...] = ()
    max_model_calls: int = 2
    max_execution_seconds: int = 600


class StoryGraphBundle:
    _ALLOWED_PATHS = (
        "NOTICE.md",
        "SKILL.md",
        "references/continuity-review.md",
        "references/entity-reconciliation.md",
        "references/episode-segmentation.md",
        "references/interaction-continuity.md",
        "references/production-entities.md",
        "references/reference-brief.md",
        "references/reference-planning.md",
        "references/scene-facts.md",
        "references/scene-occurrences.md",
        "references/scene-structure.md",
        "references/script-spans.md",
        "references/shot-detail.md",
        "references/source-evidence.md",
        "references/story-analysis.md",
        "references/storyboard-table.md",
        "references/structure-identity-review.md",
        "references/visual-identity.md",
    )
    _KNOWN_PATHS = _ALLOWED_PATHS

    def __init__(self, repository_root: Path | None = None) -> None:
        root = repository_root or Path(__file__).resolve().parents[4]
        self.root = root / "agent" / "skills" / "build-storygraph"
        self.manifest = BundleManifest()

    @classmethod
    def allowed_paths(cls) -> tuple[str, ...]:
        return cls._ALLOWED_PATHS

    @classmethod
    def known_paths(cls) -> tuple[str, ...]:
        return cls._KNOWN_PATHS

    def compute_hash(self) -> str:
        if self.root.is_symlink() or not self.root.is_dir():
            raise BundleInvalid("StoryGraph bundle root is invalid")
        actual: set[str] = set()
        for path in self.root.rglob("*"):
            if path.is_symlink():
                raise BundleInvalid("StoryGraph bundle contains a symlink")
            if path.is_file():
                actual.add(path.relative_to(self.root).as_posix())
        if actual != set(self._KNOWN_PATHS):
            raise BundleInvalid("StoryGraph bundle file set is invalid")

        files: list[dict[str, object]] = []
        for relative_path in self._ALLOWED_PATHS:
            path = self.root / relative_path
            try:
                content = path.read_bytes()
                content.decode("utf-8")
            except (OSError, UnicodeDecodeError) as error:
                raise BundleInvalid("StoryGraph bundle contains invalid UTF-8") from error
            files.append(
                {
                    "path": relative_path,
                    "byte_length": len(content),
                    "sha256": hashlib.sha256(content).hexdigest(),
                }
            )
        notice_hash = next(str(file["sha256"]) for file in files if file["path"] == "NOTICE.md")
        provenance_hash = _canonical_hash(
            {
                "contract_id": "storygraph-bundle-provenance-production",
                "origin": "project_owned",
                "source_url": "https://github.com/StephenQiu30/lanverse",
                "license_spdx": "MIT",
                "notice_hash": notice_hash,
            }
        )
        isolation_hash = _canonical_hash(
            {
                "contract_id": "storygraph-bundle-filesystem-isolation-production",
                "rules": ["exact_file_set", "no_symlink", "relative_posix_path", "utf8"],
                "files": files,
            }
        )
        return _canonical_hash(
            {
                "contract_id": "storygraph-bundle-content-production",
                "bundle_entrypoint": "SKILL.md",
                "bundle_file_manifest": files,
                "provenance_manifest_hash": provenance_hash,
                "notice_hash": notice_hash,
                "isolation_scan_hash": isolation_hash,
            }
        )

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


def _canonical_hash(value: object) -> str:
    encoded = json.dumps(
        value,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    ).encode("utf-8")
    return hashlib.sha256(encoded).hexdigest()
