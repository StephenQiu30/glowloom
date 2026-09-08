from __future__ import annotations

import ast
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]


def test_shared_contracts_and_reasoning_do_not_depend_on_runtime_layers() -> None:
    boundaries = {
        "protocol": ("app.harness", "app.creation", "app.modules", "app.reasoning"),
        "text_contract": ("app.harness", "app.creation", "app.modules", "app.reasoning"),
        "reasoning": ("app.harness", "app.creation", "app.modules"),
        "creation": ("app.modules", "app.reasoning"),
        "modules/text_storyboard": (
            "app.harness",
            "app.creation",
            "app.modules.storygraph",
        ),
    }
    for directory, forbidden in boundaries.items():
        for path in (ROOT / "app" / directory).rglob("*.py"):
            for node in ast.walk(ast.parse(path.read_text())):
                imports: list[str] = []
                if isinstance(node, ast.ImportFrom) and node.module:
                    imports.append(node.module)
                elif isinstance(node, ast.Import):
                    imports.extend(alias.name for alias in node.names)
                assert not any(
                    name == prefix or name.startswith(prefix + ".")
                    for name in imports
                    for prefix in forbidden
                ), (path, imports)


def test_agent_image_contains_trusted_and_service_source_packages(tmp_path: Path) -> None:
    import shutil

    image = (ROOT / "Dockerfile").read_text()
    assert "agent/app/api" in image
    assert "agent/app/skills" in image
    assert "agent/app/harness" in image
    assert "agent/app/reasoning" in image
    target = tmp_path / "app"
    target.mkdir()
    shutil.copyfile(ROOT / "app/__init__.py", target / "__init__.py")
    shutil.copyfile(ROOT / "app/main.py", target / "main.py")
    for package in (
        "api",
        "core",
        "skills",
        "protocol",
        "text_contract",
        "creation",
        "harness",
        "modules",
        "reasoning",
    ):
        assert f"agent/app/{package} ./app/{package}" in image
        shutil.copytree(
            ROOT / "app" / package, target / package, ignore=shutil.ignore_patterns("__pycache__")
        )
    result = subprocess.run(
        [
            sys.executable,
            "-c",
            (
                "import os; os.environ.update({"
                "'CREATION_DATABASE_URL': 'postgresql://creation:test@localhost/creation', "
                "'CREATION_AGENT_SECRET': 'synthetic-creation-secret-at-least-32-bytes', "
                "'AGENT_EXECUTION_SECRET': 'synthetic-execution-secret-at-least-32-bytes'}); "
                "import app.main, app.creation.temporal"
            ),
        ],
        cwd=tmp_path,
        capture_output=True,
        text=True,
        check=False,
    )
    assert result.returncode == 0, result.stderr
