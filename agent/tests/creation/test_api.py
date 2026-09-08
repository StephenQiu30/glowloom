import json
import time

import httpx

from app.creation.repository import Repository
from app.main import create_app
from tests.creation.test_contract import SECRET, authorization
from tests.creation.test_repository import new_command


async def test_signed_http_accept_lookup_conflict_and_authentication(
    repository: Repository,
) -> None:
    app = create_app(repository, SECRET, "creation-text")
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://test"
    ) as client:
        command = new_command()
        body = command.model_dump_json(by_alias=True).encode()
        endpoint = "/internal/creation/commands"
        headers = {
            "X-Lanverse-Creation-Authorization": authorization(body, expires=int(time.time()) + 60)
        }
        response = await client.post(endpoint, content=body, headers=headers)
        assert response.status_code == 202
        receipt = response.json()
        repeated = await client.post(endpoint, content=body, headers=headers)
        assert repeated.status_code == 202 and repeated.json() == receipt
        assert "schema_" not in receipt
        lookup = endpoint + "/" + command.command_id
        lookup_headers = {
            "X-Lanverse-Creation-Authorization": authorization(
                b"", lookup, "GET", int(time.time()) + 60
            )
        }
        response = await client.get(lookup, headers=lookup_headers)
        assert response.status_code == 200 and response.json() == receipt
        assert (await client.get(lookup)).status_code == 401
        assert (
            await client.post(endpoint, content=body + b" ", headers=headers)
        ).status_code == 401
        changed = json.loads(body)
        changed["source"]["content_hash"] = "b" * 64
        changed_body = json.dumps(changed).encode()
        headers["X-Lanverse-Creation-Authorization"] = authorization(
            changed_body, expires=int(time.time()) + 60
        )
        assert (
            await client.post(endpoint, content=changed_body, headers=headers)
        ).status_code == 409
        assert (await client.get("/readyz")).status_code == 200


async def test_invalid_body_is_rejected_without_persisting(repository: Repository) -> None:
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=create_app(repository, SECRET, "queue")),
        base_url="http://test",
    ) as client:
        for body in [b"{} {}", b'{"command_id":"a","command_id":"b"}', b"{}"]:
            headers = {
                "X-Lanverse-Creation-Authorization": authorization(
                    body, expires=int(time.time()) + 60
                )
            }
            response = await client.post(
                "/internal/creation/commands", content=body, headers=headers
            )
            assert response.status_code == 422
            assert "input" not in response.text and "body" not in response.text
        response = await client.post("/internal/creation/commands", content=b"x" * 16385)
        assert response.status_code == 413
        assert await repository.claim() is None


async def test_agent_runtime_readiness_gate_is_checked_after_creation_storage(
    repository: Repository,
) -> None:
    app = create_app(repository, SECRET, "queue", runtime_ready=lambda: False)
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://test"
    ) as client:
        response = await client.get("/readyz")
    assert response.status_code == 503
    assert response.json() == {"detail": "agent_runtime_unavailable"}


async def test_unavailable_storage_does_not_return_acceptance() -> None:
    from unittest.mock import AsyncMock

    from psycopg import OperationalError

    repository = AsyncMock()
    repository.accept.side_effect = OperationalError("synthetic sensitive database diagnostic")
    repository.ready.side_effect = OperationalError("synthetic sensitive database diagnostic")
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=create_app(repository, SECRET, "queue")),
        base_url="http://test",
    ) as client:
        command = new_command()
        body = command.model_dump_json(by_alias=True).encode()
        headers = {
            "X-Lanverse-Creation-Authorization": authorization(body, expires=int(time.time()) + 60)
        }
        response = await client.post("/internal/creation/commands", content=body, headers=headers)
        assert response.status_code == 503
        assert response.json() == {"detail": "creation_storage_unavailable"}
        assert (await client.get("/readyz")).status_code == 503


async def test_execution_and_full_draft_survive_reconnect_and_require_exact_signature(
    repository: Repository,
) -> None:
    from app.creation.execution import InvocationLease
    from tests.creation.test_execution import result_for, setup_execution

    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    result = await store.finish(lease, await result_for(task))
    await store.progress(run, "waiting_review", "map_manuscript")
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=create_app(Repository(repository.dsn), SECRET, "queue")),
        base_url="http://test",
    ) as client:
        endpoint = f"/internal/creation/commands/{run}/execution"

        def headers(path: str) -> dict[str, str]:
            return {
                "X-Lanverse-Creation-Authorization": authorization(
                    b"", path, "GET", int(time.time()) + 60
                )
            }

        assert (await client.get(endpoint)).status_code == 401
        snapshot = await client.get(endpoint, headers=headers(endpoint))
        assert snapshot.status_code == 200
        assert snapshot.json()["status"] == "waiting_review"
        assert snapshot.json()["reserved_calls"] == 1
        draft_id = snapshot.json()["outputs"][0]["draft_id"]
        draft_endpoint = f"/internal/creation/commands/{run}/drafts/{draft_id}"
        assert (await client.get(draft_endpoint, headers=headers(endpoint))).status_code == 401
        draft = await client.get(draft_endpoint, headers=headers(draft_endpoint))
        assert draft.status_code == 200
        assert draft.json()["schema"] == "creation-draft-production"
        assert draft.json()["revision"] == 1 and draft.json()["result"] == result
        assert draft.json()["task"] == task.model_dump(mode="json")


async def test_unknown_execution_cannot_request_resume(repository: Repository) -> None:
    from tests.creation.test_execution import setup_execution

    store, run, _ = await setup_execution(repository)
    await store.progress(run, "blocked", "map_manuscript", "harness_response_unknown")
    command = await repository.command(run)
    assert command is not None
    endpoint = f"/internal/creation/commands/{run}/resume"
    body = json.dumps({"payload_hash": command.payload_hash}).encode()
    headers = {
        "X-Lanverse-Creation-Authorization": authorization(
            body, endpoint, "POST", int(time.time()) + 60
        )
    }
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=create_app(repository, SECRET, "queue")),
        base_url="http://test",
    ) as http:
        assert (await http.post(endpoint, content=body, headers=headers)).status_code == 409
    snapshot = await store.snapshot(run)
    assert not snapshot["can_resume"] and snapshot["reserved_calls"] == 0
