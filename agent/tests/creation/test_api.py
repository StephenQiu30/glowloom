import json
import time

import httpx

from app.creation.api import create_app
from app.creation.repository import Repository
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
