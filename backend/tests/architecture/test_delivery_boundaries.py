import ast
from pathlib import Path

MODULES = Path(__file__).resolve().parents[2] / "app/modules"
FORBIDDEN_CONTRACT_DEPENDENCIES = (
    "fastapi",
    "sqlalchemy",
    "app.integrations",
)
FORBIDDEN_MODULE_SUFFIXES = (".api", ".models", ".repository", ".service")


def _import_targets(source: Path) -> list[str]:
    targets: list[str] = []
    for node in ast.walk(ast.parse(source.read_text(encoding="utf-8"))):
        if isinstance(node, ast.Import):
            targets.extend(alias.name for alias in node.names)
        elif isinstance(node, ast.ImportFrom) and node.module:
            targets.append(node.module)
    return targets


def test_public_contracts_are_framework_and_persistence_free() -> None:
    offenders: list[str] = []
    for source in MODULES.glob("*/contracts.py"):
        for target in _import_targets(source):
            if target.startswith(FORBIDDEN_CONTRACT_DEPENDENCIES) or target.endswith(
                FORBIDDEN_MODULE_SUFFIXES
            ):
                offenders.append(f"{source.parent.name}:{target}")
    assert offenders == []
