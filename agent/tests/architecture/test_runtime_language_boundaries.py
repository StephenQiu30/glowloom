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


def test_agent_entrypoint_mounts_the_single_agent_service() -> None:
    entrypoint = (AGENT_ROOT / "app/main.py").read_text(encoding="utf-8")
    dockerfile = (AGENT_ROOT / "Dockerfile").read_text(encoding="utf-8")
    assert "from app.api.application import create_agent_app" in entrypoint
    assert "app.main:create_agent_app" in dockerfile
    assert "agent/app/creation" in dockerfile
    assert "agent/app/candidate_runtime" in dockerfile


def test_candidate_runtime_cannot_load_trusted_storage_or_orchestration() -> None:
    project = (AGENT_ROOT / "pyproject.toml").read_text(encoding="utf-8")
    for dependency in (
        "sqlalchemy",
        "asyncpg",
        "redis",
        "aiokafka",
        "minio",
        "elasticsearch",
        "runware",
    ):
        assert dependency not in project.casefold()
    probe = subprocess.run(
        [
            sys.executable,
            "-c",
            (
                "import sys; import app.candidate_runtime.api; "
                "assert not any(name.split('.')[0] in ('sqlalchemy', 'psycopg', 'temporalio') "
                "or name.startswith('app.creation') "
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
    assert (BACKEND_ROOT / "api/openapi/lanverse-public-api.json").is_file()
    assert not (AGENT_ROOT / "openapi.json").exists()


def test_agent_image_contains_all_internal_capabilities() -> None:
    image = (AGENT_ROOT / "Dockerfile").read_text()
    base_requirements = (AGENT_ROOT / "requirements.txt").read_text()
    assert "agent/app/creation" in image
    assert "agent/app/modules" in image
    assert "@openai/codex" in image
    assert "psycopg" not in base_requirements and "temporalio" not in base_requirements
