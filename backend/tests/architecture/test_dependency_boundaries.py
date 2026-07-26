from __future__ import annotations

import ast
import json
import re
import tomllib
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
FRONTEND = ROOT / "frontend"
PACKAGE = BACKEND / "src"
DOMAIN = PACKAGE / "domain"
LEGACY_IMPORT_ROOTS = {
    "bootstrap",
    "entrypoints",
    "infrastructure",
    "jobs",
    "modules",
    "shared_kernel",
}

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
    for path in python_files(DOMAIN):
        for module in imported_modules(path):
            if module.split(".")[0] in DOMAIN_BANNED_IMPORTS:
                violations.append(f"{path.relative_to(ROOT)} -> {module}")
    assert not violations, violations


def test_source_does_not_import_legacy_package_roots() -> None:
    violations: list[str] = []
    for path in python_files(PACKAGE):
        for module in imported_modules(path):
            parts = module.split(".")
            if parts[0] == "lanverse" or parts[0] in LEGACY_IMPORT_ROOTS:
                violations.append(f"{path.relative_to(ROOT)} -> {module}")
    assert not violations, violations


def test_services_and_repositories_do_not_depend_on_http_layer() -> None:
    violations: list[str] = []
    for layer in ("services", "repositories"):
        for path in python_files(PACKAGE / layer):
            for module in imported_modules(path):
                if module == "api" or module.startswith("api."):
                    violations.append(f"{path.relative_to(ROOT)} -> {module}")
    assert not violations, violations


def test_repositories_do_not_depend_on_services_or_workers() -> None:
    violations: list[str] = []
    for path in python_files(PACKAGE / "repositories"):
        for module in imported_modules(path):
            if module == "services" or module.startswith(("services.", "workers", "workers.")):
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
        ):
            violations.append(relative.as_posix())
    assert not violations, violations


def test_generated_openapi_client_uses_the_native_api_directory() -> None:
    source = FRONTEND / "src"
    api = source / "api"
    assert api.is_dir()
    assert list(api.glob("*.ts"))
    assert not [path for path in api.iterdir() if path.is_dir()]
    assert not (source / "services" / "generated").exists()
    assert not (source / "services" / "api").exists()


def test_only_api_wrapper_may_import_generated_services() -> None:
    source = FRONTEND / "src"
    wrapper = source / "store" / "backend-api.ts"
    assert wrapper.is_file()
    wrapper_text = wrapper.read_text()
    assert "@/api/" in wrapper_text
    assert "@/lib/request" not in wrapper_text
    assert not re.search(r"\b(fetch|XMLHttpRequest)\s*\(|\baxios\b", wrapper_text)
    generated_modules = {
        path.stem for path in (source / "api").glob("*.ts") if path.name != "typings.d.ts"
    }
    violations: list[str] = []
    for path in sorted(source.rglob("*.ts")) + sorted(source.rglob("*.tsx")):
        relative = path.relative_to(source)
        if relative.parent == Path("api"):
            continue
        text = path.read_text()
        for module in generated_modules:
            if f"@/api/{module}" in text and relative != Path("store/backend-api.ts"):
                violations.append(f"{relative.as_posix()} -> {module}")
    assert not violations, violations
