from pathlib import Path

AGENT_ROOT = Path(__file__).resolve().parents[2]


def test_fastapi_application_has_one_canonical_factory() -> None:
    main = (AGENT_ROOT / "app/main.py").read_text(encoding="utf-8")
    application = AGENT_ROOT / "app/api/application.py"
    router = AGENT_ROOT / "app/api/router.py"
    dockerfile = (AGENT_ROOT / "Dockerfile").read_text(encoding="utf-8")

    assert application.is_file()
    assert router.is_file()
    assert "from app.api.application import create_agent_app" in main
    assert "app.main:create_agent_app" in dockerfile
    assert "app.creation.api:create_agent_app" not in dockerfile


def test_creation_module_does_not_own_the_canonical_application_factory() -> None:
    source = (AGENT_ROOT / "app/creation/api.py").read_text(encoding="utf-8")
    assert "def create_configured_app" in source
    assert "def create_agent_app" in source
    assert "from app.api.application import create_agent_app as factory" in source
