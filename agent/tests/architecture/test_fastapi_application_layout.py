from pathlib import Path

AGENT_ROOT = Path(__file__).resolve().parents[2]


def test_fastapi_application_uses_the_standard_single_factory() -> None:
    main = (AGENT_ROOT / "app/main.py").read_text(encoding="utf-8")
    router = (AGENT_ROOT / "app/api/router.py").read_text(encoding="utf-8")
    dockerfile = (AGENT_ROOT / "Dockerfile").read_text(encoding="utf-8")

    assert "def create_app" in main
    assert "FastAPI(" in main
    assert "lifespan=lifespan" in main
    assert "include_router" in router
    assert "app.main:create_app" in dockerfile
    assert "--factory" in dockerfile
    assert not (AGENT_ROOT / "app/api/application.py").exists()
    assert not (AGENT_ROOT / "app/creation/api.py").exists()


def test_http_transport_is_split_from_business_modules() -> None:
    production = list((AGENT_ROOT / "app").rglob("*.py"))
    route_files = {path.relative_to(AGENT_ROOT / "app").as_posix() for path in production}
    assert {
        "api/dependencies.py",
        "api/routes/health.py",
        "api/routes/creation.py",
        "api/routes/storygraph.py",
        "api/routes/scene_analysis.py",
        "api/routes/text_storyboard.py",
    } <= route_files
    for path in production:
        relative = path.relative_to(AGENT_ROOT / "app").as_posix()
        if relative.startswith("api/") or relative == "main.py":
            continue
        source = path.read_text(encoding="utf-8")
        assert "add_api_route" not in source, path
        assert "lifespan_context" not in source, path
