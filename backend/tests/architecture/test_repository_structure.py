from __future__ import annotations

import os
import tomllib
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
FRONTEND = ROOT / "frontend"

BACKEND_MODULES = {
    "project_catalog",
    "story_development",
    "generation",
    "production_jobs",
    "media_library",
    "delivery",
}
FRONTEND_FEATURES = {"projects", "story", "studio", "tasks", "delivery"}
FORBIDDEN_ROOTS = {"deploy", "build", "data", "env", "src", "apps", "packages"}
MODULE_PARTS = {"domain", "application", "infrastructure", "transport"}


def child_directories(path: Path) -> set[str]:
    if not path.exists():
        return set()
    return {item.name for item in path.iterdir() if item.is_dir()}


def test_application_roots_exist_without_forbidden_roots() -> None:
    assert BACKEND.is_dir()
    assert FRONTEND.is_dir()
    assert not (child_directories(ROOT) & FORBIDDEN_ROOTS)


def test_backend_scaffold_files_are_complete() -> None:
    required = {
        "README.md",
        "pyproject.toml",
        "uv.lock",
        ".python-version",
    }
    assert required <= {item.name for item in BACKEND.iterdir()}


def test_frontend_scaffold_files_are_complete() -> None:
    required = {
        ".gitignore",
        ".node-version",
        "package.json",
        "pnpm-lock.yaml",
        "pnpm-workspace.yaml",
        "components.json",
        "AGENTS.md",
    }
    assert required <= {item.name for item in FRONTEND.iterdir()}


def test_backend_console_entries_are_exact() -> None:
    with (BACKEND / "pyproject.toml").open("rb") as stream:
        project = tomllib.load(stream)["project"]
    assert project["scripts"] == {
        "lanverse-api": "lanverse.entrypoints.api:main",
        "lanverse-worker": "lanverse.entrypoints.worker:main",
    }


def test_present_backend_modules_follow_allowlist_and_shape() -> None:
    modules_root = BACKEND / "src" / "lanverse" / "modules"
    present = child_directories(modules_root) - {"__pycache__"}
    assert present <= BACKEND_MODULES
    if os.getenv("LANVERSE_ARCHITECTURE_FINAL") == "1":
        assert present == BACKEND_MODULES

    for name in present:
        module = modules_root / name
        assert MODULE_PARTS <= child_directories(module)
        assert (module / "public.py").is_file()


def test_present_frontend_features_follow_allowlist() -> None:
    features_root = FRONTEND / "src" / "features"
    present = child_directories(features_root)
    assert present <= FRONTEND_FEATURES
    if os.getenv("LANVERSE_ARCHITECTURE_FINAL") == "1":
        assert present == FRONTEND_FEATURES
