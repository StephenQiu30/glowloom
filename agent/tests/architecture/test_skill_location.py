from __future__ import annotations

import json
from pathlib import Path
from typing import Any, cast

from app.modules.storygraph.bundle import StoryGraphBundle

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
AGENT_ROOT = REPOSITORY_ROOT / "agent"
SKILLS_ROOT = AGENT_ROOT / "skills"
LEGACY_SKILLS_ROOT = REPOSITORY_ROOT / ".agents" / "skills"


def test_build_storygraph_is_the_only_agent_owned_skill_bundle() -> None:
    assert {path.name for path in SKILLS_ROOT.iterdir() if path.is_dir()} == {"build-storygraph"}
    assert not LEGACY_SKILLS_ROOT.exists()

    fixture = cast(
        dict[str, Any],
        json.loads(
            (REPOSITORY_ROOT / "backend/tests/fixtures/agent/storygraph-definition.json").read_text(
                encoding="utf-8"
            )
        ),
    )
    bundle = StoryGraphBundle(REPOSITORY_ROOT)
    assert list(bundle.allowed_paths()) == fixture["bundle_paths"]
    assert bundle.compute_hash() == fixture["skill_bundle_hash"]
    assert not list(SKILLS_ROOT.rglob("openai.yaml"))


def test_runtime_and_image_have_no_old_skill_loader_or_fallback() -> None:
    source = "\n".join(
        path.read_text(encoding="utf-8") for path in sorted((AGENT_ROOT / "app").rglob("*.py"))
    )
    dockerfile = (AGENT_ROOT / "Dockerfile").read_text(encoding="utf-8")
    dockerignore = (REPOSITORY_ROOT / ".dockerignore").read_text(encoding="utf-8")
    assert 'rglob("*.md")' not in source
    assert ".agents" not in source
    assert "COPY --chown=lanverse:lanverse agent/skills ./skills" in dockerfile
    assert ".agents" not in dockerfile
    assert ".agents" not in dockerignore

    dependency_files = (
        (AGENT_ROOT / "pyproject.toml").read_text(encoding="utf-8")
        + (AGENT_ROOT / "requirements.txt").read_text(encoding="utf-8")
    ).casefold()
    assert "langgraph" not in dependency_files
    assert "langchain" not in dependency_files

    tracked_python = {
        path.relative_to(AGENT_ROOT).as_posix() for path in (AGENT_ROOT / "app").rglob("*.py")
    }
    assert not any(
        path.startswith(prefix)
        for path in tracked_python
        for prefix in (
            "app/modules/scripts/",
            "app/modules/skills/",
            "app/modules/storyboards/",
        )
    )
