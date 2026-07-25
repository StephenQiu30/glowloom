from __future__ import annotations

import re
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]

REQUIRED_TARGETS = {
    "test-architecture",
    "test-migration",
    "contracts-check",
    "test-jobs",
    "test-e2e",
    "lint",
    "typecheck",
    "build",
}


def test_makefile_exposes_required_project_commands() -> None:
    makefile = ROOT / "Makefile"
    assert makefile.is_file()
    text = makefile.read_text()
    targets = set(re.findall(r"^([a-z][a-z0-9-]*):", text, flags=re.MULTILINE))
    assert targets >= REQUIRED_TARGETS

    result = subprocess.run(
        ["make", "-n", *sorted(REQUIRED_TARGETS)],
        cwd=ROOT,
        check=False,
        capture_output=True,
        text=True,
    )
    assert result.returncode == 0, result.stderr


def test_all_container_stages_pin_base_image_digests() -> None:
    for relative in (Path("backend/Dockerfile"), Path("frontend/Dockerfile")):
        dockerfile = ROOT / relative
        assert dockerfile.is_file()
        from_lines = [
            line.strip() for line in dockerfile.read_text().splitlines() if line.startswith("FROM ")
        ]
        assert from_lines
        assert all("@sha256:" in line for line in from_lines), from_lines
        assert all(":latest" not in line for line in from_lines), from_lines
