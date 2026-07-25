from __future__ import annotations

import ast
import json
import re
import tomllib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
FRONTEND = ROOT / "frontend"
MODULES = BACKEND / "src" / "lanverse" / "modules"

DOMAIN_BANNED_IMPORTS = {
    "alembic",
    "asyncpg",
    "fastapi",
    "langchain",
    "langchain_core",
    "langgraph",
    "minio",
    "pydantic",
}
BACKEND_BANNED_PACKAGES = {
    "celery",
    "langchain",
    "langgraph",
    "psycopg",
    "redis",
    "sqlalchemy",
    "temporalio",
}
FRONTEND_BANNED_PACKAGES = {
    "@tanstack/react-query",
    "axios",
    "redux-persist",
    "zustand",
}


def python_files(path: Path) -> list[Path]:
    return sorted(path.rglob("*.py")) if path.exists() else []


def imported_modules(path: Path) -> list[str]:
    tree = ast.parse(path.read_text(), filename=str(path))
    modules: list[str] = []
    for node in ast.walk(tree):
        if isinstance(node, ast.Import):
            modules.extend(alias.name for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module:
            modules.append(node.module)
    return modules


def dependency_name(specifier: str) -> str:
    match = re.match(r"[A-Za-z0-9_.-]+", specifier)
    assert match
    return match.group(0).lower().replace("_", "-")


def test_domains_do_not_import_frameworks_or_adapters() -> None:
    violations: list[str] = []
    for path in python_files(MODULES):
        if "domain" not in path.relative_to(MODULES).parts:
            continue
        for module in imported_modules(path):
            if module.split(".")[0] in DOMAIN_BANNED_IMPORTS:
                violations.append(f"{path.relative_to(ROOT)} -> {module}")
    assert not violations, violations


def test_cross_module_imports_use_public_contracts() -> None:
    violations: list[str] = []
    for path in python_files(MODULES):
        relative = path.relative_to(MODULES)
        owner = relative.parts[0]
        for module in imported_modules(path):
            parts = module.split(".")
            if parts[:2] != ["lanverse", "modules"] or len(parts) < 3:
                continue
            target = parts[2]
            if target != owner and parts[3:] != ["public"]:
                violations.append(f"{path.relative_to(ROOT)} -> {module}")
    assert not violations, violations


def test_backend_dependencies_exclude_forbidden_platforms() -> None:
    with (BACKEND / "pyproject.toml").open("rb") as stream:
        config = tomllib.load(stream)
    specs = list(config["project"].get("dependencies", []))
    for group in config.get("dependency-groups", {}).values():
        specs.extend(group)
    names = {dependency_name(specifier) for specifier in specs}
    assert not (names & BACKEND_BANNED_PACKAGES)


def test_frontend_dependencies_exclude_duplicate_state_and_http_clients() -> None:
    package = json.loads((FRONTEND / "package.json").read_text())
    names = set(package.get("dependencies", {})) | set(package.get("devDependencies", {}))
    assert not (names & FRONTEND_BANNED_PACKAGES)


def test_frontend_has_only_one_direct_http_sender() -> None:
    source = FRONTEND / "src"
    allowed = {Path("lib/request.ts")}
    violations: list[str] = []
    for path in sorted(source.rglob("*.ts")) + sorted(source.rglob("*.tsx")):
        relative = path.relative_to(source)
        text = path.read_text()
        if (
            re.search(r"\b(fetch|XMLHttpRequest)\s*\(|\baxios\b", text)
            and relative not in allowed
            and "services/generated" not in relative.as_posix()
        ):
            violations.append(relative.as_posix())
    assert not violations, violations


def test_only_api_wrapper_imports_generated_services() -> None:
    source = FRONTEND / "src"
    violations: list[str] = []
    for path in sorted(source.rglob("*.ts")) + sorted(source.rglob("*.tsx")):
        relative = path.relative_to(source)
        if "services/generated" in path.read_text() and relative != Path("services/api.ts"):
            violations.append(relative.as_posix())
    assert not violations, violations
