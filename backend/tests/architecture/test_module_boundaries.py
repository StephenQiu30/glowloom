import ast
from pathlib import Path

MODULES = Path(__file__).resolve().parents[2] / "app/modules"
REPOSITORY = Path(__file__).resolve().parents[3]
KNOWN_INTERNAL_IMPORTS: set[str] = set()

SOURCE_ROOTS = (
    REPOSITORY / "backend/app",
    REPOSITORY / "backend/tests",
    REPOSITORY / "frontend/src",
    REPOSITORY / "frontend/tests",
)
SOURCE_SUFFIXES = {".py", ".ts", ".tsx"}
FORBIDDEN_SOURCE_STEMS = {
    "common",
    "data",
    "helpers",
    "manager",
    "misc",
    "processor",
}
MAX_SOURCE_FILE_NAME_LENGTH = 64

FORBIDDEN_CONTRACT_IMPORTS = (
    "fastapi",
    "sqlalchemy",
    "app.integrations",
)
FORBIDDEN_DATA_METHODS = {"commit", "load", "publish", "save", "send"}
FORBIDDEN_DATA_SUFFIXES = ("Bean", "DTO", "Data", "Info", "Object")


def _base_name(base: ast.expr) -> str:
    if isinstance(base, ast.Name):
        return base.id
    if isinstance(base, ast.Attribute):
        return base.attr
    return ""


def _is_true(node: ast.expr) -> bool:
    return isinstance(node, ast.Constant) and node.value is True


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


def test_source_file_names_are_semantic_and_short() -> None:
    offenders: set[str] = set()
    for root in SOURCE_ROOTS:
        for source in root.rglob("*"):
            if not source.is_file() or source.suffix not in SOURCE_SUFFIXES:
                continue
            relative = source.relative_to(REPOSITORY)
            if not source.name.isascii():
                offenders.add(f"{relative}:non-ascii")
            if len(source.name) > MAX_SOURCE_FILE_NAME_LENGTH:
                offenders.add(f"{relative}:longer-than-64")
            if source.stem in FORBIDDEN_SOURCE_STEMS:
                offenders.add(f"{relative}:non-semantic-name")
    assert offenders == set()


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


def test_contract_data_objects_are_immutable_and_have_no_io_methods() -> None:
    offenders: set[str] = set()
    for source in MODULES.glob("*/contracts.py"):
        tree = ast.parse(source.read_text(encoding="utf-8"))
        relative = source.relative_to(MODULES)
        for node in tree.body:
            if not isinstance(node, ast.ClassDef):
                continue
            bases = {_base_name(base) for base in node.bases}
            if node.name.endswith(FORBIDDEN_DATA_SUFFIXES):
                offenders.add(f"{relative}:{node.name}:non-semantic-name")
            if bases & {"Exception", "Protocol", "RuntimeError", "StrEnum"}:
                continue
            methods = {
                item.name
                for item in node.body
                if isinstance(item, (ast.AsyncFunctionDef, ast.FunctionDef))
            }
            for method in methods & FORBIDDEN_DATA_METHODS:
                offenders.add(f"{relative}:{node.name}.{method}:io-method")
            dataclass_decorators = [
                decorator
                for decorator in node.decorator_list
                if isinstance(decorator, ast.Call)
                and _base_name(decorator.func) == "dataclass"
            ]
            for decorator in dataclass_decorators:
                options = {keyword.arg: keyword.value for keyword in decorator.keywords}
                if not _is_true(options.get("frozen", ast.Constant(False))):
                    offenders.add(f"{relative}:{node.name}:mutable-dataclass")
                if not _is_true(options.get("slots", ast.Constant(False))):
                    offenders.add(f"{relative}:{node.name}:unslotted-dataclass")
    assert offenders == set()
