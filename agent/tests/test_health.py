from fastapi.testclient import TestClient

from app.config import Settings
from app.main_api import create_app


def test_internal_health() -> None:
    client = TestClient(create_app(Settings()))

    response = client.get("/internal/health")

    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_public_docs_are_disabled() -> None:
    client = TestClient(create_app(Settings()))

    assert client.get("/docs").status_code == 404
