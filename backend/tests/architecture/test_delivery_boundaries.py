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


def _model_classes(module: str) -> set[str]:
    source = MODULES / module / "models.py"
    tree = ast.parse(source.read_text(encoding="utf-8"))
    return {node.name for node in tree.body if isinstance(node, ast.ClassDef)}


def test_public_contracts_are_framework_and_persistence_free() -> None:
    offenders: list[str] = []
    for source in MODULES.glob("*/contracts.py"):
        for target in _import_targets(source):
            if target.startswith(FORBIDDEN_CONTRACT_DEPENDENCIES) or target.endswith(
                FORBIDDEN_MODULE_SUFFIXES
            ):
                offenders.append(f"{source.parent.name}:{target}")
    assert offenders == []


def test_task_and_message_delivery_remain_current_flat_lifecycles() -> None:
    production = MODULES / "production"
    messaging = MODULES / "messaging"

    assert not (production / "tasks").exists()
    assert not (messaging / "outbox").exists()
    assert not (messaging / "inbox").exists()
    assert _model_classes("production") == {"Task"}
    assert _model_classes("messaging") == {"InboxDelivery", "OutboxEvent"}


def test_public_delivery_results_have_no_schema_reexport_layer() -> None:
    production_contracts = (MODULES / "production/contracts.py").read_text()
    production_schemas = (MODULES / "production/schemas.py").read_text()

    assert "class TaskResponse" in production_contracts
    assert "class TaskResponse" not in production_schemas
    assert "ScriptExtractionTaskCommand" not in production_schemas
    assert "TaskStatus" not in production_schemas
    assert not (MODULES / "messaging/schemas.py").exists()
