from fastapi import Query
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


def test_internal_route_errors_use_stable_problem_contract() -> None:
    client = TestClient(create_app(Settings()))

    missing = client.get("/internal/missing", headers={"X-Request-Id": "agent-probe-1"})
    assert missing.status_code == 404
    assert missing.headers["content-type"] == "application/problem+json"
    assert missing.headers["x-request-id"] == "agent-probe-1"
    assert missing.json() == {
        "type": "https://lanverse.local/errors/not_found",
        "title": "Not found",
        "status": 404,
        "code": "not_found",
        "request_id": "agent-probe-1",
    }

    wrong_method = client.post("/internal/health")
    assert wrong_method.status_code == 405
    assert wrong_method.headers["content-type"] == "application/problem+json"
    assert wrong_method.headers["allow"] == "GET"
    assert wrong_method.json()["code"] == "method_not_allowed"
    assert wrong_method.json()["request_id"] == wrong_method.headers["x-request-id"]


def test_internal_validation_error_omits_input_and_rejects_invalid_request_id() -> None:
    app = create_app(Settings())

    @app.get("/internal/test-validation")
    def require_limit(limit: int = Query(ge=1)) -> dict[str, int]:
        return {"limit": limit}

    client = TestClient(app)
    response = client.get(
        "/internal/test-validation?limit=private-value",
        headers={"X-Request-Id": "invalid request id"},
    )

    assert response.status_code == 400
    assert response.headers["content-type"] == "application/problem+json"
    assert response.json()["code"] == "validation_failed"
    assert response.json()["request_id"] == response.headers["x-request-id"]
    assert response.json()["request_id"] != "invalid request id"
    assert "private-value" not in response.text


def test_unexpected_internal_error_does_not_leak_exception() -> None:
    app = create_app(Settings())

    @app.get("/internal/test-failure")
    def fail() -> None:
        raise RuntimeError("secret-token-from-dependency")

    client = TestClient(app, raise_server_exceptions=False)
    response = client.get("/internal/test-failure", headers={"X-Request-Id": "agent-probe-2"})

    assert response.status_code == 500
    assert response.headers["content-type"] == "application/problem+json"
    assert response.headers["x-request-id"] == "agent-probe-2"
    assert response.json() == {
        "type": "https://lanverse.local/errors/internal",
        "title": "Internal server error",
        "status": 500,
        "code": "internal",
        "request_id": "agent-probe-2",
    }
    assert "secret-token-from-dependency" not in response.text
