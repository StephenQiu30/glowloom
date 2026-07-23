from __future__ import annotations

import ast
from pathlib import Path


BUSINESS_MODULES = ("identity", "catalog")
FRAMEWORKS = {"boto3", "celery", "fastapi", "minio", "sqlalchemy"}
FRAMEWORK_FREE_FILES = {
    "catalog/model.py",
    "catalog/ports.py",
    "identity/invitations.py",
    "identity/model.py",
    "identity/ports.py",
    "identity/sessions.py",
}
SOURCE_DIRECTORIES = {"api", "catalog", "identity", "infrastructure", "__pycache__"}


def required_layout_errors(root: Path) -> list[str]:
    required = (
        ".node-version",
        ".python-version",
        "Makefile",
        "frontend/components.json",
        "frontend/package.json",
        "frontend/pnpm-lock.yaml",
        "frontend/postcss.config.mjs",
        "frontend/src/app/page.tsx",
        "frontend/src/components/ui/button.tsx",
        "backend/pyproject.toml",
        "backend/uv.lock",
        "backend/src/thief/__init__.py",
        "backend/src/thief/api/app.py",
        "backend/src/thief/catalog/model.py",
        "backend/src/thief/identity/model.py",
        "backend/src/thief/infrastructure/unit_of_work.py",
        "backend/src/thief/scheduler.py",
        "backend/src/thief/settings.py",
        "backend/src/thief/worker.py",
    )
    errors = [
        f"missing:{path}" for path in required if not (root / path).is_file()
    ]
    errors.extend(
        f"obsolete:{path}"
        for path in ("backend/apps", "backend/packages")
        if (root / path).exists()
    )
    source = root / "backend/src/thief"
    if source.is_dir():
        errors.extend(unexpected_module_errors(source))
    return errors


def unexpected_module_errors(source: Path) -> list[str]:
    return [
        f"unexpected-module:{path.name}"
        for path in sorted(source.iterdir())
        if path.is_dir() and path.name not in SOURCE_DIRECTORIES
    ]


def find_violations(source: Path) -> list[str]:
    violations: list[str] = []
    for path in sorted(source.rglob("*.py")):
        relative = path.relative_to(source)
        try:
            tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
        except SyntaxError as error:
            violations.append(f"invalid-python:{relative}:{error.lineno}")
            continue
        for node in ast.walk(tree):
            imported = _imported_module(node)
            if imported:
                violations.extend(_import_violations(relative, imported))
    return violations


def _import_violations(relative: Path, imported: str) -> list[str]:
    relative_name = relative.as_posix()
    imported_root = imported.split(".", 1)[0]
    if relative_name in FRAMEWORK_FREE_FILES and imported_root in FRAMEWORKS:
        return [f"business-framework:{relative}:{imported}"]

    if not relative.parts or relative.parts[0] not in BUSINESS_MODULES:
        return []
    current = relative.parts[0]
    for other in BUSINESS_MODULES:
        if other != current and imported.startswith(f"thief.{other}"):
            return [f"cross-business-module:{relative}:{imported}"]
    return []


def _imported_module(node: ast.AST) -> str | None:
    if isinstance(node, ast.Import) and node.names:
        return node.names[0].name
    if isinstance(node, ast.ImportFrom):
        return node.module
    return None
