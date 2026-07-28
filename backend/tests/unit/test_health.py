from fastapi.testclient import TestClient

from app.main import create_app


def test_health_endpoint_returns_request_id() -> None:
    with TestClient(create_app()) as client:
        response = client.get("/healthz", headers={"x-request-id": "test-request"})
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}
    assert response.headers["x-request-id"] == "test-request"
