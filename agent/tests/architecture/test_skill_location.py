from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import cast

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
AGENT_ROOT = REPOSITORY_ROOT / "agent"
SKILLS_ROOT = AGENT_ROOT / "skills"
LEGACY_SKILLS_ROOT = REPOSITORY_ROOT / ".agents" / "skills"
MANIFEST_PATH = AGENT_ROOT / "tests/fixtures/skills/legacy-skill-manifest-v1.json"

EXPECTED_SKILLS = {
    "analyze-scene",
    "draft-shots",
    "extract-bible-evidence",
    "plan-scene",
    "reconcile-bible",
    "repair-shots",
    "review-bible",
    "review-shots",
}


def _expected_manifest() -> dict[str, str]:
    manifest = cast(dict[str, object], json.loads(MANIFEST_PATH.read_text(encoding="utf-8")))
    assert manifest["version"] == 1
    return cast(dict[str, str], manifest["files"])


def _actual_manifest() -> dict[str, str]:
    return {
        path.relative_to(SKILLS_ROOT).as_posix(): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in sorted(SKILLS_ROOT.rglob("*"))
        if path.is_file()
    }


def test_eight_skills_keep_the_accepted_paths_and_bytes() -> None:
    assert {path.name for path in SKILLS_ROOT.iterdir() if path.is_dir()} == EXPECTED_SKILLS
    assert _actual_manifest() == _expected_manifest()


def test_runtime_has_one_agent_owned_skill_path_without_legacy_fallback() -> None:
    assert not LEGACY_SKILLS_ROOT.exists()

    harness = (AGENT_ROOT / "app/modules/skills/harness.py").read_text(encoding="utf-8")
    dockerfile = (AGENT_ROOT / "Dockerfile").read_text(encoding="utf-8")
    dockerignore = (REPOSITORY_ROOT / ".dockerignore").read_text(encoding="utf-8")
    assert 'self._repository_root / "agent" / "skills"' in harness
    assert ".agents" not in harness
    assert "COPY --chown=lanverse:lanverse agent/skills ./skills" in dockerfile
    assert ".agents" not in dockerfile
    assert ".agents" not in dockerignore
