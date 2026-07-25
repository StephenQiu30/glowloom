from __future__ import annotations

from datetime import UTC, datetime
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient
from pydantic import ValidationError

from lanverse.bootstrap.api import create_app
from lanverse.shared_kernel.http_contracts import (
    Problem,
    TaskAccepted,
    TaskProgress,
    TaskResponse,
)
from lanverse.shared_kernel.http_errors import HttpProblem
from lanverse.shared_kernel.http_headers import (
    parse_if_match,
    strong_etag,
    validate_idempotency_key,
)


def test_problem_contract_is_strict_and_safe() -> None:
    request_id = uuid4()
    problem = Problem(
        type="https://lanverse.local/problems/version-conflict",
        title="Version conflict",
        status=412,
        code="VERSION_CONFLICT",
        retryable=False,
        request_id=request_id,
    )

    assert problem.model_dump(mode="json")["request_id"] == str(request_id)
    with pytest.raises(ValidationError):
        Problem.model_validate({**problem.model_dump(), "stack": "secret"})


def test_async_acceptance_and_task_polling_contracts_are_stable() -> None:
    task_id = uuid4()
    accepted = TaskAccepted(
        task_id=task_id,
        status="queued",
        resource_version=1,
        status_url=f"/v1/tasks/{task_id}",
    )
    task = TaskResponse(
        id=task_id,
        type="generate_script",
        scope={"episode_id": str(uuid4())},
        status="running",
        progress=TaskProgress(phase="provider", completed=1, total=3),
        input_outdated=False,
        result_refs=[],
        resource_version=2,
        created_at=datetime.now(UTC),
        updated_at=datetime.now(UTC),
    )

    assert accepted.status == "queued"
    assert task.poll_after_ms == 2000
    assert task.error is None
    assert task.finished_at is None


@pytest.mark.parametrize("value", ["short", "contains space", "x" * 129])
def test_idempotency_key_rejects_invalid_values(value: str) -> None:
    with pytest.raises(HttpProblem) as raised:
        validate_idempotency_key(value)
    assert raised.value.status == 422
    assert raised.value.code == "INVALID_IDEMPOTENCY_KEY"


def test_strong_etag_rejects_weak_or_stale_versions() -> None:
    assert strong_etag(7) == '"7"'
    assert parse_if_match('"7"') == 7

    for value in ("7", 'W/"7"', '"8", "9"'):
        with pytest.raises(HttpProblem) as raised:
            parse_if_match(value)
        assert raised.value.code == "INVALID_IF_MATCH"


def test_app_factory_maps_problems_and_applies_exact_cors() -> None:
    app = create_app()

    @app.get("/__contract_problem__")
    async def contract_problem() -> None:
        raise HttpProblem(status=409, title="Conflict", code="TEST_CONFLICT")

    with TestClient(app) as client:
        response = client.get("/__contract_problem__")
        preflight = client.options(
            "/v1/projects",
            headers={
                "Origin": "http://127.0.0.1:3000",
                "Access-Control-Request-Method": "POST",
                "Access-Control-Request-Headers": "Idempotency-Key,Content-Type",
            },
        )
        rejected = client.options(
            "/v1/projects",
            headers={
                "Origin": "http://example.com",
                "Access-Control-Request-Method": "POST",
            },
        )

    assert response.status_code == 409
    assert response.headers["content-type"].startswith("application/problem+json")
    assert response.json()["code"] == "TEST_CONFLICT"
    assert response.json()["request_id"]
    assert preflight.headers["access-control-allow-origin"] == "http://127.0.0.1:3000"
    assert preflight.headers["access-control-allow-methods"] == "GET, POST, PUT"
    assert "etag" in preflight.headers["access-control-expose-headers"].lower()
    assert "access-control-allow-origin" not in rejected.headers
