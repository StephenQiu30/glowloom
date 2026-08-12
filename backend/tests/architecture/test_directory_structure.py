import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


def test_user_facing_documents_and_e2e_specs_use_semantic_filenames() -> None:
    slice_token = re.compile(r"(^|[-_])s\d+", re.IGNORECASE)
    candidates = [
        *(ROOT / "docs").rglob("*.md"),
        *(ROOT / "frontend/tests/e2e").glob("*.spec.ts"),
    ]

    assert [
        path.relative_to(ROOT).as_posix()
        for path in candidates
        if slice_token.search(path.name)
    ] == []


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


def test_environment_specific_compose_supports_robust_full_stack_startup() -> None:
    assert (ROOT / "backend/Dockerfile").is_file()
    assert (ROOT / "frontend/Dockerfile").is_file()
    assert (ROOT / "docker-compose.yml").is_file()
    assert (ROOT / "docker-compose.prod.yml").is_file()
    assert not (ROOT / "docker-compose-env.yml").exists()

    compose = (ROOT / "docker-compose.yml").read_text()
    production_compose = (ROOT / "docker-compose.prod.yml").read_text()

    for service in ("postgres", "redis", "rabbitmq", "minio", "server", "web"):
        assert f"  {service}:" in compose
    for image in (
        "postgres:latest",
        "redis:latest",
        "rabbitmq:latest",
        "minio/minio:latest",
    ):
        assert f"image: {image}" in compose
    assert "@sha256:" not in compose
    assert "container_name:" not in compose
    assert compose.count("healthcheck:") == 6
    assert compose.count("restart: unless-stopped") == 6
    assert "condition: service_healthy" in compose
    assert "@postgres:5432/" in compose
    assert "redis://redis:6379/0" in compose
    assert "@rabbitmq:5672/" in compose
    assert "MINIO_ENDPOINT: minio:9000" in compose
    assert "python -m app.initialize_database && exec python -m app.server" in compose
    assert production_compose.count("ports: !reset []") == 4
    assert production_compose.count("build: !reset null") == 2
    assert "ENVIRONMENT: production" in production_compose

    assert not (ROOT / "Makefile").exists()
    assert not (ROOT / "backend/tools").exists()
    readme = (ROOT / "README.md").read_text()
    workflow = (ROOT / ".github/workflows/ci.yml").read_text()
    assert "docker compose up -d --build --wait" in readme
    assert "up -d --no-build --pull always --wait" in readme
    assert "docker compose --env-file .env.example build server web" in workflow
    assert (ROOT / "backend/app/cache_admin.py").is_file()

    backend_dockerfile = (ROOT / "backend/Dockerfile").read_text()
    assert 'CMD ["python", "-m", "app.server"]' in backend_dockerfile


def test_environment_configuration_has_one_repository_entrypoint() -> None:
    assert (ROOT / ".env.example").is_file()
    assert (ROOT / ".env.production.example").is_file()
    assert not (ROOT / "backend/.env.example").exists()
    assert not (ROOT / "frontend/.env.example").exists()

    package = json.loads((ROOT / "frontend/package.json").read_text())
    assert "--env-file-if-exists=../.env" in package["scripts"]["openapi2ts"]

    compose = (ROOT / "docker-compose.yml").read_text()
    assert "ARK_API_KEY: ${ARK_API_KEY:-}" in compose

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

    workflow = (ROOT / ".github/workflows/ci.yml").read_text()
    dockerfile = (ROOT / "backend/Dockerfile").read_text()
    playwright = (ROOT / "frontend/playwright.config.ts").read_text()
    assert "setup-uv" not in workflow
    assert "python -m venv backend/.venv" in workflow
    assert "backend/.venv/bin/python -m pip install 'pip==26.1.2'" in workflow
    assert "backend/.venv/bin/python -m pip install --requirement" in workflow
    assert "uv sync" not in workflow
    assert "uv run" not in workflow
    assert "ghcr.io/astral-sh/uv" not in dockerfile
    assert "uv run" not in playwright


def test_ci_executes_the_real_media_stack_contract() -> None:
    workflow = (ROOT / ".github/workflows/ci.yml").read_text()
    browser_job = workflow.split("\n  browser:\n", maxsplit=1)[1].split(
        "\n  delivery:\n", maxsplit=1
    )[0]

    assert "name: Start MinIO contract service" in workflow
    assert "minio/minio:latest server /data" in workflow
    assert "minio/minio:RELEASE." not in workflow
    assert "@sha256:" not in workflow
    assert "ffprobe -version" in workflow
    assert "LANVERSE_RUN_MEDIA_STACK_CONTRACT=1" in workflow
    assert "name: Stop MinIO contract service" in workflow
    assert "if: always()" in workflow
    assert "docker rm --force lanverse-ci-minio" in workflow
    assert "image: redis:latest" in workflow
    assert "LANVERSE_RUN_REDIS_CONTRACT=1" in workflow
    assert "sudo apt-get install --yes ffmpeg" in browser_job
    assert "ffprobe -version" in browser_job


def test_ci_exposes_project_execution_boundaries_and_a_stable_required_gate() -> None:
    workflow = (ROOT / ".github/workflows/ci.yml").read_text()

    expected_jobs = {
        "backend": "Backend / Quality",
        "frontend": "Frontend / Quality",
        "openapi": "API / OpenAPI Drift",
        "external_contracts": "Infrastructure / Contracts",
        "browser": "System / Browser E2E",
        "delivery": "Delivery / Compose and Images",
        "required": "Required / CI",
    }
    for job_id, job_name in expected_jobs.items():
        assert f"\n  {job_id}:\n" in workflow
        assert f"name: {job_name}" in workflow

    assert "permissions:\n  contents: read" in workflow
    assert "merge_group:" in workflow
    assert "\n    paths:" not in workflow
    assert workflow.count("timeout-minutes:") == len(expected_jobs)

    required = workflow.split("\n  required:\n", maxsplit=1)[1]
    assert "if: ${{ always() }}" in required
    for dependency in expected_jobs.keys() - {"required"}:
        assert f"      - {dependency}\n" in required
        assert f"${{{{ needs.{dependency}.result }}}}" in required

    assert "LANVERSE_RUN_FFPROBE_CONTRACT=1" in workflow
    assert "LANVERSE_RUN_MINIO_CONTRACT=1" in workflow


def test_scripts_is_split_by_capability() -> None:
    scripts = ROOT / "backend/app/modules/scripts"
    for capability in ("versions", "extractions", "structure"):
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
    assert "include_router(structure_router)" in api


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


def test_integration_database_and_client_fixtures_are_shared() -> None:
    tests = ROOT / "backend/tests"
    conftest = tests / "conftest.py"
    assert conftest.is_file()
    fixture_source = conftest.read_text()
    assert "async def session_factory" in fixture_source
    assert "async def client" in fixture_source

    duplicate_definitions: list[Path] = []
    for path in (tests / "integration").rglob("test_*.py"):
        source = path.read_text()
        if "async def session_factory" in source or "async def client(" in source:
            duplicate_definitions.append(path)
    assert not duplicate_definitions


def test_projects_and_identity_integration_tests_follow_capability_boundaries() -> None:
    integration = ROOT / "backend/tests/integration"
    expected = (
        integration / "identity/authentication/test_authentication_api.py",
        integration / "identity/workspaces/test_workspaces_api.py",
        integration / "projects/projects/test_projects_api.py",
        integration / "projects/episodes/test_episodes_api.py",
        integration / "projects/snapshots/test_snapshots_api.py",
    )
    assert all(path.is_file() for path in expected)
    assert not (integration / "test_identity_api.py").exists()
    assert not (integration / "test_projects_api.py").exists()
    assert (ROOT / "backend/tests/support/project_builders.py").is_file()
