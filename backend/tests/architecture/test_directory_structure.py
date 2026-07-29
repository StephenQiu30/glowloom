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

    assert "  api:" in business_compose
    assert "  web:" in business_compose
    assert "  io-worker:" in business_compose
    assert business_compose.count("image: lanverse-backend:local") == 3
    assert 'command: ["python", "-m", "app.io_worker"]' in business_compose
    assert "  postgres:" not in business_compose
    assert "  redis:" not in business_compose
    assert "  rabbitmq:" not in business_compose
    assert "  minio:" not in business_compose

    assert "  api:" not in environment_compose
    assert "  web:" not in environment_compose
    for service in ("postgres", "redis", "rabbitmq", "minio"):
        assert f"  {service}:" in environment_compose


def test_environment_configuration_has_one_repository_entrypoint() -> None:
    assert (ROOT / ".env.example").is_file()
    assert not (ROOT / "backend/.env.example").exists()
    assert not (ROOT / "frontend/.env.example").exists()

    package = json.loads((ROOT / "frontend/package.json").read_text())
    assert "--env-file-if-exists=../.env" in package["scripts"]["openapi2ts"]

    next_config = (ROOT / "frontend/next.config.ts").read_text()
    assert "process.loadEnvFile(repositoryEnvironmentFile)" in next_config
    assert 'resolve(process.cwd(), "../.env")' in next_config
