from pathlib import Path

REPOSITORY_ROOT = Path(__file__).resolve().parents[3]
BACKEND_ROOT = REPOSITORY_ROOT / "backend"
AGENT_ROOT = REPOSITORY_ROOT / "agent"


def test_backend_is_a_single_go_module_without_python_runtime_files() -> None:
    assert (BACKEND_ROOT / "go.mod").is_file()
    assert (BACKEND_ROOT / "cmd/api/main.go").is_file()
    assert not list(BACKEND_ROOT.rglob("*.py"))
    assert not (BACKEND_ROOT / "pyproject.toml").exists()


def test_migrating_python_runtime_lives_under_agent() -> None:
    assert (AGENT_ROOT / "pyproject.toml").is_file()
    assert (AGENT_ROOT / "app/main.py").is_file()
    assert not (AGENT_ROOT / "go.mod").exists()
