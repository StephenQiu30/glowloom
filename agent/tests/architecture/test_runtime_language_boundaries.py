import subprocess
import sys
from pathlib import Path

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
BACKEND_ROOT = REPOSITORY_ROOT / "backend"
AGENT_ROOT = REPOSITORY_ROOT / "agent"


def test_backend_is_the_only_public_business_runtime() -> None:
    assert (BACKEND_ROOT / "go.mod").is_file()
    command_entries = list((BACKEND_ROOT / "cmd").iterdir())
    assert command_entries == [BACKEND_ROOT / "cmd/main.go"]
    assert not list(BACKEND_ROOT.rglob("*.py"))
    assert not (BACKEND_ROOT / "pyproject.toml").exists()


def test_agent_entrypoint_only_mounts_candidate_runtime() -> None:
    entrypoint = (AGENT_ROOT / "app/main.py").read_text(encoding="utf-8")
    dockerfile = (AGENT_ROOT / "Dockerfile").read_text(encoding="utf-8")
    assert "from app.candidate_runtime.api import app" in entrypoint
    assert "app.candidate_runtime.api:app" in dockerfile
    assert "app.runtime.api" not in dockerfile


def test_agent_runtime_has_no_business_storage_dependencies() -> None:
    project = (AGENT_ROOT / "pyproject.toml").read_text(encoding="utf-8")
    for dependency in (
        "sqlalchemy",
        "asyncpg",
        "redis",
        "aiokafka",
        "minio",
        "elasticsearch",
        "temporalio",
        "runware",
    ):
        assert dependency not in project.casefold()
    probe = subprocess.run(
        [
            sys.executable,
            "-c",
            (
                "import sys; import app.candidate_runtime.api; "
                "assert not any(name == 'sqlalchemy' or name.startswith('sqlalchemy.') "
                "for name in sys.modules)"
            ),
        ],
        cwd=AGENT_ROOT,
        check=False,
        capture_output=True,
        text=True,
    )
    assert probe.returncode == 0, probe.stderr


def test_public_openapi_contract_is_owned_by_backend() -> None:
    assert (BACKEND_ROOT / "api/openapi/lanverse-v1.json").is_file()
    assert not (AGENT_ROOT / "openapi.json").exists()
