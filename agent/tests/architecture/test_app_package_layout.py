from pathlib import Path

APP_PACKAGE = Path(__file__).resolve().parents[2] / "app"


def test_app_package_root_contains_only_the_composition_entrypoint() -> None:
    root_modules = {path.name for path in APP_PACKAGE.glob("*.py")}

    assert root_modules == {"__init__.py", "main.py"}
