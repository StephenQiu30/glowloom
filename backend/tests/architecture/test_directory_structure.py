import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


def test_production_and_test_code_are_separate() -> None:
    assert (ROOT / "backend/app").is_dir()
    assert (ROOT / "backend/tests").is_dir()
    assert not list((ROOT / "backend/app").rglob("test_*.py"))
    assert not list((ROOT / "frontend/src").rglob("*.test.*"))
    assert not list((ROOT / "frontend/src").rglob("*.spec.*"))


def test_forbidden_overdesigned_directories_do_not_exist() -> None:
    forbidden = [
        ROOT / "backend/src/lanverse",
        ROOT / "backend/app/app",
        ROOT / "deploy",
        ROOT / "shared",
        ROOT / "common",
        ROOT / "packages",
    ]
    assert not [path for path in forbidden if path.exists()]


def test_container_files_live_at_their_runtime_boundaries() -> None:
    assert (ROOT / "backend/Dockerfile").is_file()
    assert (ROOT / "frontend/Dockerfile").is_file()
    assert (ROOT / "docker-compose.yml").is_file()
    assert (ROOT / "docker-compose-env.yml").is_file()

    business_compose = (ROOT / "docker-compose.yml").read_text()
    environment_compose = (ROOT / "docker-compose-env.yml").read_text()

    assert "name: lanverse-services" in business_compose
    assert "  server:" in business_compose
    assert "  web:" in business_compose
    assert "  api:" not in business_compose
    assert "  scheduler:" not in business_compose
    assert "  io-worker:" not in business_compose
    assert business_compose.count("image: lanverse-backend:local") == 1
    assert "  postgres:" not in business_compose
    assert "  redis:" not in business_compose
    assert "  rabbitmq:" not in business_compose
    assert "  minio:" not in business_compose

    for container_name in ("lanverse-server", "lanverse-web"):
        assert f"container_name: {container_name}" in business_compose

    assert "name: lanverse-environment" in environment_compose
    assert "  server:" not in environment_compose
    assert "  web:" not in environment_compose
    for service in ("postgres", "redis", "rabbitmq", "minio"):
        assert f"  {service}:" in environment_compose
        assert f"container_name: lanverse-{service}" in environment_compose
    for image in ("postgres:latest", "redis:latest", "rabbitmq:latest", "minio/minio:latest"):
        assert f"image: {image}\n" in environment_compose
    assert "@sha256:" not in environment_compose

    makefile = (ROOT / "Makefile").read_text()
    assert "services-up:" in makefile
    assert "services-down:" in makefile
    assert "docker compose -f docker-compose.yml up -d --build" in makefile
    assert "business-up:" not in makefile
    assert "business-down:" not in makefile

    backend_dockerfile = (ROOT / "backend/Dockerfile").read_text()
    assert 'CMD ["python", "-m", "app.server"]' in backend_dockerfile


def test_environment_configuration_has_one_repository_entrypoint() -> None:
    assert (ROOT / ".env.example").is_file()
    assert not (ROOT / "backend/.env.example").exists()
    assert not (ROOT / "frontend/.env.example").exists()

    package = json.loads((ROOT / "frontend/package.json").read_text())
    assert "--env-file-if-exists=../.env" in package["scripts"]["openapi2ts"]

    next_config = (ROOT / "frontend/next.config.ts").read_text()
    assert "process.loadEnvFile(repositoryEnvironmentFile)" in next_config
    assert 'resolve(process.cwd(), "../.env")' in next_config


def test_backend_uses_pycharm_venv_and_pip_lock_baseline() -> None:
    assert (ROOT / "backend/test_main.http").is_file()
    assert (ROOT / "backend/requirements.txt").is_file()
    assert (ROOT / "backend/requirements-dev.txt").is_file()
    assert not (ROOT / "backend/uv.lock").exists()

    pyproject = (ROOT / "backend/pyproject.toml").read_text()
    assert "[tool.uv]" not in pyproject
    assert "[project.optional-dependencies]" in pyproject
    assert 'venvPath = "."' in pyproject
    assert 'venv = ".venv"' in pyproject

    makefile = (ROOT / "Makefile").read_text()
    assert "PYTHON ?= python3.11" in makefile
    assert "$(PYTHON) -m venv backend/.venv" in makefile
    assert "$(VENV_PYTHON) -m pip install 'pip==26.1.2'" in makefile
    assert "uv sync" not in makefile
    assert "uv run" not in makefile

    workflow = (ROOT / ".github/workflows/ci.yml").read_text()
    dockerfile = (ROOT / "backend/Dockerfile").read_text()
    playwright = (ROOT / "frontend/playwright.config.ts").read_text()
    assert "setup-uv" not in workflow
    assert "ghcr.io/astral-sh/uv" not in dockerfile
    assert "uv run" not in playwright


def test_scripts_is_split_by_versions_and_extractions_capabilities() -> None:
    scripts = ROOT / "backend/app/modules/scripts"
    for capability in ("versions", "extractions"):
        package = scripts / capability
        assert (package / "__init__.py").is_file()
        assert (package / "api.py").is_file()
        assert (package / "schemas.py").is_file()
        assert (package / "service.py").is_file()

    assert not (scripts / "schemas.py").exists()
    assert not (scripts / "service.py").exists()

    api = (scripts / "api.py").read_text()
    assert "include_router(versions_router)" in api
    assert "include_router(extractions_router)" in api


def test_projects_is_split_by_project_episode_and_snapshot_capabilities() -> None:
    module = ROOT / "backend/app/modules/projects"
    for capability in ("projects", "episodes", "snapshots"):
        package = module / capability
        assert (package / "__init__.py").is_file()
        assert (package / "api.py").is_file()
        assert (package / "schemas.py").is_file()
        assert (package / "service.py").is_file()

    assert not (module / "schemas.py").exists()
    assert not (module / "service.py").exists()

    api = (module / "api.py").read_text()
    assert "include_router(projects_router)" in api
    assert "include_router(episodes_router)" in api
    assert "include_router(snapshots_router)" in api


def test_identity_is_split_by_authentication_and_workspaces_capabilities() -> None:
    module = ROOT / "backend/app/modules/identity"
    for capability in ("authentication", "workspaces"):
        package = module / capability
        assert (package / "__init__.py").is_file()
        assert (package / "api.py").is_file()
        assert (package / "schemas.py").is_file()
        assert (package / "service.py").is_file()

    assert not (module / "schemas.py").exists()
    assert not (module / "service.py").exists()

    api = (module / "api.py").read_text()
    assert "include_router(authentication_router)" in api
    assert "include_router(workspaces_router)" in api
