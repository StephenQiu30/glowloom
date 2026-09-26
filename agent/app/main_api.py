"""agent-api: internal HTTP entry of the Agent service (OPS-01 §3, port 8090)."""

from fastapi import FastAPI

from app.config import Settings


def create_app(settings: Settings | None = None) -> FastAPI:
    settings = settings or Settings()
    # Internal-only service: no public OpenAPI docs.
    app = FastAPI(title="lanverse-agent", docs_url=None, redoc_url=None, openapi_url=None)
    app.state.settings = settings

    @app.get("/internal/health")
    def health() -> dict[str, str]:
        return {"status": "ok"}

    return app
