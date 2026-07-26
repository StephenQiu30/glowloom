from __future__ import annotations

from datetime import UTC, datetime
from uuid import uuid4

import pytest
from httpx import ASGITransport, AsyncClient
from pydantic import ValidationError

from api.headers import (
    parse_if_match,
    strong_etag,
    validate_idempotency_key,
)
from api.problems import HttpProblem
from main import create_app
from schemas.common import (
    Problem,
    TaskAccepted,
    TaskProgress,
    TaskResponse,
)


def test_problem_contract_is_strict_and_safe() -> None:
    request_id = uuid4()
    problem = Problem(
        type="https://local/problems/version-conflict",
        title="Version conflict",
        status=412,
        code="VERSION_CONFLICT",
        retryable=False,
        request_id=request_id,
    )

    assert problem.model_dump(mode="json")["request_id"] == str(request_id)
    with pytest.raises(ValidationError):
        Problem.model_validate({**problem.model_dump(), "stack": "secret"})


def test_global_error_status_enum_builds_problem_responses() -> None:
    from api.responses import ApiErrorStatus, error_responses

    assert {status.value for status in ApiErrorStatus} == {404, 409, 412, 422, 503}
    assert error_responses(
        ApiErrorStatus.NOT_FOUND,
        ApiErrorStatus.CONFLICT,
    ) == {404: {"model": Problem}, 409: {"model": Problem}}


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
        result_refs=(),
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


@pytest.mark.asyncio
async def test_app_factory_maps_problems_and_applies_exact_cors() -> None:
    app = create_app()

    @app.get("/__contract_problem__")
    async def contract_problem() -> None:
        raise HttpProblem(status=409, title="Conflict", code="TEST_CONFLICT")

    async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
        response = await client.get(
            "/__contract_problem__", headers={"Origin": "http://127.0.0.1:3000"}
        )
        preflight = await client.options(
            "/v1/projects",
            headers={
                "Origin": "http://127.0.0.1:3000",
                "Access-Control-Request-Method": "POST",
                "Access-Control-Request-Headers": "Idempotency-Key,Content-Type",
            },
        )
        rejected = await client.options(
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
    assert "etag" in response.headers["access-control-expose-headers"].lower()
    assert "access-control-allow-origin" not in rejected.headers
