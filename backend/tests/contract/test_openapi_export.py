from __future__ import annotations

import json
import subprocess
import sys
from pathlib import Path

BACKEND = Path(__file__).resolve().parents[2]
SCRIPT = BACKEND / "scripts" / "export_openapi.py"


def export(output: Path, *, check: bool = False) -> subprocess.CompletedProcess[str]:
    command = [sys.executable, str(SCRIPT), "--output", str(output)]
    if check:
        command.append("--check")
    return subprocess.run(
        command,
        cwd=BACKEND,
        check=False,
        capture_output=True,
        text=True,
    )


def test_openapi_export_is_deterministic_and_uses_the_app_factory(tmp_path: Path) -> None:
    first = tmp_path / "first.json"
    second = tmp_path / "second.json"

    first_result = export(first)
    second_result = export(second)

    assert first_result.returncode == 0, first_result.stderr
    assert second_result.returncode == 0, second_result.stderr
    assert first.read_bytes() == second.read_bytes()
    document = json.loads(first.read_text())
    assert document["openapi"] == "3.1.0"
    assert document["info"] == {"title": "Lanverse API", "version": "0.1.0"}
    operations = {
        operation["operationId"]
        for path in document["paths"].values()
        for operation in path.values()
        if isinstance(operation, dict) and "operationId" in operation
    }
    assert operations == {
        "createProject",
        "listProjects",
        "getProject",
        "getEpisode",
        "createSourceRevision",
        "confirmSource",
        "listSourceRevisions",
        "getSourceRevision",
    }


def test_openapi_check_detects_artifact_drift(tmp_path: Path) -> None:
    artifact = tmp_path / "openapi.json"
    assert export(artifact).returncode == 0
    assert export(artifact, check=True).returncode == 0

    artifact.write_text("{}\n")
    drift = export(artifact, check=True)

    assert drift.returncode == 1
    assert "OpenAPI artifact is out of date" in drift.stderr
