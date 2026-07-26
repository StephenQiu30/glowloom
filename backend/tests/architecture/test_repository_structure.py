from __future__ import annotations

import os
import tomllib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
FRONTEND = ROOT / "frontend"

BACKEND_SOURCE = BACKEND / "src"
BACKEND_LAYERS = {
    "api",
    "core",
    "db",
    "domain",
    "integrations",
    "repositories",
    "resources",
    "schemas",
    "services",
    "workers",
}
LEGACY_LAYERS = {
    "bootstrap",
    "entrypoints",
    "infrastructure",
    "jobs",
    "modules",
    "shared_kernel",
}
FRONTEND_FEATURES = {"projects", "story", "studio", "tasks", "delivery"}
FORBIDDEN_ROOTS = {"deploy", "build", "data", "env", "src", "apps", "packages"}


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


def test_frontend_dependency_build_scripts_are_explicitly_reviewed() -> None:
    workspace = (FRONTEND / "pnpm-workspace.yaml").read_text()

    assert "ignoredBuiltDependencies" not in workspace
    assert "  es5-ext: false" in workspace
    assert {
        line.strip().removesuffix(": true")
        for line in workspace.splitlines()
        if line.startswith("  ") and line.endswith(": true")
    } == {"sharp", "unrs-resolver"}


def test_backend_console_entries_are_exact() -> None:
    with (BACKEND / "pyproject.toml").open("rb") as stream:
        project = tomllib.load(stream)["project"]
    assert project["scripts"] == {
        "lanverse-api": "main:run",
        "lanverse-worker": "worker:run",
    }


def test_backend_builds_the_direct_src_layout() -> None:
    with (BACKEND / "pyproject.toml").open("rb") as stream:
        config = tomllib.load(stream)

    assert config["build-system"] == {
        "requires": ["setuptools==83.0.0"],
        "build-backend": "setuptools.build_meta",
    }
    setuptools = config["tool"]["setuptools"]
    assert setuptools["package-dir"] == {"": "src"}
    assert setuptools["py-modules"] == ["main", "worker"]
    assert setuptools["include-package-data"] is True
    assert setuptools["packages"]["find"] == {
        "where": ["src"],
        "namespaces": False,
    }


def test_backend_uses_one_fastapi_technical_layer_set() -> None:
    present = {
        name
        for name in child_directories(BACKEND_SOURCE)
        if name != "__pycache__" and not name.endswith(".egg-info")
    }
    assert present == BACKEND_LAYERS
    assert not (present & LEGACY_LAYERS)
    assert "lanverse" not in present
    assert {path.name for path in BACKEND_SOURCE.iterdir() if path.is_file()} == {
        "main.py",
        "worker.py",
    }


def test_present_frontend_features_follow_allowlist() -> None:
    features_root = FRONTEND / "src" / "features"
    present = child_directories(features_root)
    assert present <= FRONTEND_FEATURES
    if os.getenv("LANVERSE_ARCHITECTURE_FINAL") == "1":
        assert present == FRONTEND_FEATURES


def test_tailwind_inline_theme_uses_parse_time_font_families() -> None:
    theme = (FRONTEND / "src" / "app" / "globals.css").read_text()
    assert '--font-sans: "Geist", "Geist Fallback", ui-sans-serif' in theme
    assert '--font-mono: "Geist Mono", "Geist Mono Fallback", ui-monospace' in theme
    assert "--font-sans: var(--font-geist-sans)" not in theme


def test_api_errors_have_one_global_registry_and_response_enum() -> None:
    api = BACKEND_SOURCE / "api"
    assert (api / "errors.py").is_file()
    assert (api / "responses.py").is_file()
    assert not list(api.glob("*_errors.py"))
    violations = [
        path.relative_to(ROOT).as_posix()
        for path in (api / "routes").glob("*.py")
        if "ERROR_RESPONSES" in path.read_text()
    ]
    assert not violations, violations
