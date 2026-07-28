import ast
from pathlib import Path

APP = Path(__file__).resolve().parents[2] / "app"


def test_core_does_not_import_business_modules() -> None:
    offenders: list[str] = []
    for source in (APP / "core").glob("*.py"):
        tree = ast.parse(source.read_text(encoding="utf-8"))
        for node in ast.walk(tree):
            if isinstance(node, ast.ImportFrom) and (node.module or "").startswith("app.modules"):
                offenders.append(f"{source.name}:{node.lineno}")
    assert offenders == []
