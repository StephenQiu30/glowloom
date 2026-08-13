import ast
from pathlib import Path

MODULES = Path(__file__).resolve().parents[2] / "app/modules"
KNOWN_INTERNAL_IMPORTS: set[str] = set()

FORBIDDEN_CONTRACT_IMPORTS = (
    "fastapi",
    "sqlalchemy",
    "app.integrations",
)


def _cross_module_internal_imports() -> set[str]:
    offenders: set[str] = set()
    for source in MODULES.rglob("*.py"):
        relative = source.relative_to(MODULES)
        source_module = relative.parts[0]
        tree = ast.parse(source.read_text(encoding="utf-8"))
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                for alias in node.names:
                    parts = alias.name.split(".")
                    if (
                        len(parts) >= 3
                        and parts[:2] == ["app", "modules"]
                        and parts[2] != source_module
                    ):
                        offenders.add(f"{relative}:{alias.name}:*")
                continue
            if not isinstance(node, ast.ImportFrom):
                continue
            target = node.module or ""
            parts = target.split(".")
            if len(parts) < 3 or parts[:2] != ["app", "modules"]:
                continue
            target_module = parts[2]
            if target_module == source_module:
                continue
            imported = ",".join(sorted(alias.name for alias in node.names))
            if target == f"app.modules.{target_module}.contracts":
                continue
            if target == "app.modules.governance.audit":
                continue
            if target == f"app.modules.{target_module}" and not any(
                alias.name in {"api", "models", "repository", "service"} for alias in node.names
            ):
                continue
            offenders.add(f"{relative}:{target}:{imported}")
    return offenders


def test_cross_module_imports_use_public_contracts() -> None:
    assert _cross_module_internal_imports() == KNOWN_INTERNAL_IMPORTS


def test_public_contracts_are_plain_data_objects() -> None:
    offenders: set[str] = set()
    for source in MODULES.glob("*/contracts.py"):
        tree = ast.parse(source.read_text(encoding="utf-8"))
        for node in ast.walk(tree):
            if isinstance(node, ast.Import):
                names = [alias.name for alias in node.names]
            elif isinstance(node, ast.ImportFrom):
                names = [node.module or ""]
            else:
                continue
            for name in names:
                if name.startswith(FORBIDDEN_CONTRACT_IMPORTS):
                    offenders.add(f"{source.relative_to(MODULES)}:{name}")
    assert offenders == set()
