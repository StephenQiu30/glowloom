from __future__ import annotations

import time

import httpx
import pytest
from pydantic import BaseModel

from app.candidate_runtime.api import app
from app.modules.text_storyboard.harness import RELEASE_HASH, TextHarness, TextResult, TextTask
from app.text_contract.authorization import sign_task
from tests.unit.text_storyboard_samples import sample

SECRET = "synthetic-text-execution-secret-for-tests"


def task() -> TextTask:
    source, _, _, _, _ = sample()
    return TextTask(
        invocation_id="http-test", stage="map_manuscript", source=source, release_hash=RELEASE_HASH
    )


@pytest.mark.parametrize("case", ["missing", "expired", "too_long", "modified", "wrong_secret"])
async def test_authorization_rejects_before_inference(
    case: str, monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    value = task()
    now = int(time.time())
    expiration = now - 1 if case == "expired" else now + (120 if case == "too_long" else 30)
    token = sign_task(value, SECRET if case != "wrong_secret" else SECRET + "different", expiration)
    if case == "modified":
        value = value.model_copy(update={"invocation_id": "different-invocation"})

    async def forbidden(*_: object) -> TextResult:
        raise AssertionError("unauthorized request reached the Harness")

    monkeypatch.setattr(TextHarness, "execute", forbidden)
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://agent"
    ) as client:
        response = await client.post(
            "/internal/text-storyboard/invocations",
            json=value.model_dump(mode="json"),
            headers={} if case == "missing" else {"X-Lanverse-Text-Authorization": token},
        )
    assert response.status_code == (422 if case == "missing" else 401)


async def test_authenticated_invalid_release_never_calls_model(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    value = task().model_copy(update={"release_hash": "0" * 64})
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://agent"
    ) as client:
        response = await client.post(
            "/internal/text-storyboard/invocations",
            json=value.model_dump(mode="json"),
            headers={
                "X-Lanverse-Text-Authorization": sign_task(value, SECRET, int(time.time()) + 30)
            },
        )
    assert response.status_code == 409
    assert response.json()["detail"]["code"] == "skill_release_unavailable"
    assert response.json()["detail"]["phase"] == "preflight"


async def test_signed_request_returns_validated_draft_with_source_and_release_bindings(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    value = task()
    _, episode_map, _, _, _ = sample()

    async def reason(_: str, __: str, model: type[BaseModel], ___: int) -> BaseModel:
        assert isinstance(episode_map, model)
        return episode_map

    harness = TextHarness(reason)
    monkeypatch.setattr("app.candidate_runtime.text_storyboard_api.TextHarness", lambda: harness)
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://agent"
    ) as client:
        response = await client.post(
            "/internal/text-storyboard/invocations",
            json=value.model_dump(mode="json"),
            headers={
                "X-Lanverse-Text-Authorization": sign_task(value, SECRET, int(time.time()) + 30)
            },
        )
    assert response.status_code == 200
    result = TextResult.model_validate(response.json())
    assert result.invocation_id == value.invocation_id
    assert result.release_hash == RELEASE_HASH
    assert (
        result.status == "needs_review" and result.context.source_hash == value.source.content_hash
    )


async def test_invalid_candidate_receipt_retains_output_without_marking_it_as_draft(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    value = task()
    _, episode_map, _, _, _ = sample()
    invalid = episode_map.model_copy(deep=True)
    invalid.episodes.append(invalid.episodes[0])

    async def reason(*_: object) -> BaseModel:
        return invalid

    monkeypatch.setattr(
        "app.candidate_runtime.text_storyboard_api.TextHarness", lambda: TextHarness(reason)
    )
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://agent"
    ) as client:
        response = await client.post(
            "/internal/text-storyboard/invocations",
            json=value.model_dump(mode="json"),
            headers={
                "X-Lanverse-Text-Authorization": sign_task(value, SECRET, int(time.time()) + 30)
            },
        )
    assert response.status_code == 422
    detail = response.json()["detail"]
    assert detail["phase"] == "validation" and detail["code"] == "candidate_contract_invalid"
    assert detail["diagnostic"] == "duplicate episode key"
    assert detail["candidate"] == invalid.model_dump(mode="json")
    assert "status" not in detail


async def test_execution_error_returns_bound_unknown_receipt(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    from app.protocol.canonical import canonical_hash
    from app.reasoning.codex import CodexRuntimeUnavailable
    from app.text_contract.failure import InvocationFailure

    monkeypatch.setenv("AGENT_EXECUTION_SECRET", SECRET)
    value = task()

    async def unavailable(*_: object) -> TextResult:
        raise CodexRuntimeUnavailable("synthetic model process failure")

    monkeypatch.setattr(TextHarness, "execute", unavailable)
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://agent"
    ) as client:
        response = await client.post(
            "/internal/text-storyboard/invocations",
            json=value.model_dump(mode="json"),
            headers={
                "X-Lanverse-Text-Authorization": sign_task(value, SECRET, int(time.time()) + 30)
            },
        )
    assert response.status_code == 502
    failure = InvocationFailure.model_validate(response.json()["detail"])
    assert failure.state == "unknown" and failure.error_code == "harness_response_unknown"
    assert failure.input_hash == canonical_hash(value.model_dump(mode="json"))
    assert failure.diagnostic == "synthetic model process failure"
    assert failure.candidate is None
