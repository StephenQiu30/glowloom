import base64
import hashlib
import hmac
import json
import time
from typing import Any

import httpx
import pytest

from app.creation.platform import HarnessClient, PlatformClient, PlatformUnavailable, verify_gate
from app.text_contract.task import TextTask
from tests.creation.test_contract import SECRET
from tests.creation.test_repository import new_command
from tests.unit.text_storyboard_samples import sample


async def test_source_http_signature_binds_exact_source_and_never_follows_redirects() -> None:
    command = new_command()
    source = sample()[0]
    command = command.model_copy(
        update={
            "source": command.source.model_copy(
                update={"revision_id": source.revision_id, "content_hash": source.content_hash}
            )
        }
    )

    def handle(request: httpx.Request) -> httpx.Response:
        assert request.url.path == f"/internal/creation/runs/{command.run_id}/source"
        assert json.loads(request.content) == {"payload_hash": command.payload_hash}
        encoded, signature = request.headers["X-Lanverse-Creation-Authorization"].split(".")
        claims = json.loads(base64.urlsafe_b64decode(encoded + "=" * (-len(encoded) % 4)))
        assert claims["audience"] == "lanverse.creation.platform.source"
        assert claims["path"] == request.url.path and claims["method"] == "POST"
        assert claims["body_hash"] == hashlib.sha256(request.content).hexdigest()
        assert 0 < claims["expires_at"] - int(time.time()) <= 60
        expected = (
            base64.urlsafe_b64encode(hmac.digest(SECRET.encode(), encoded.encode(), "sha256"))
            .rstrip(b"=")
            .decode()
        )
        assert signature == expected
        return httpx.Response(200, json=source.model_dump(mode="json"))

    async with httpx.AsyncClient(transport=httpx.MockTransport(handle)) as http:
        assert await PlatformClient(http, "https://platform.test", SECRET).source(command) == source
    requests: list[str] = []

    def redirect(request: httpx.Request) -> httpx.Response:
        requests.append(str(request.url))
        return httpx.Response(307, headers={"Location": "https://other.test/secret"})

    async with httpx.AsyncClient(transport=httpx.MockTransport(redirect)) as http:
        with pytest.raises(PlatformUnavailable, match="internal_http_307"):
            await PlatformClient(http, "https://platform.test", SECRET).source(command)
    assert len(requests) == 1


async def test_harness_gets_task_signature_without_platform_credentials() -> None:
    from app.modules.text_storyboard.harness import RELEASE_HASH
    from app.text_contract.authorization import verify_task

    task = TextTask(
        invocation_id="signed-test",
        stage="map_manuscript",
        source=sample()[0],
        release_hash=RELEASE_HASH,
    )
    harness_secret = "independent-harness-secret-" * 3

    def handle(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/internal/text-storyboard/invocations"
        assert "X-Lanverse-Creation-Authorization" not in request.headers
        received = TextTask.model_validate_json(request.content)
        verify_task(
            received,
            request.headers["X-Lanverse-Text-Authorization"],
            harness_secret,
            int(time.time()),
        )
        assert SECRET not in request.content.decode()
        return httpx.Response(200, json={"synthetic": True})

    async with httpx.AsyncClient(transport=httpx.MockTransport(handle)) as http:
        assert await HarnessClient(http, "https://harness.test", harness_secret).invoke(task) == {
            "synthetic": True
        }


def test_formal_receipt_must_bind_each_candidate_and_run() -> None:
    command = new_command()
    draft: dict[str, Any] = {
        "draft_id": command.run_id,
        "step_id": command.run_id,
        "candidate_hash": "a" * 64,
        "result_hash": "b" * 64,
    }
    receipt: dict[str, Any] = {
        "schema": "creation-adoption-production",
        "run_id": command.run_id,
        "step_id": command.run_id,
        "gate": "map_manuscript",
        "proposal_revision": 1,
        "proposal_hash": draft["result_hash"],
        "candidate_hash": draft["candidate_hash"],
        "decision_id": command.run_id,
        "owner_receipts": [
            {"id": command.run_id, "owner": "script", "operation": "adopt_creation_map"}
        ],
    }
    response: dict[str, Any] = {
        "status": "accepted",
        "gate": "map_manuscript",
        "receipts": [receipt],
    }
    assert verify_gate(command, "map_manuscript", [draft], response) == response
    invalid: list[tuple[str, Any]] = [
        ("run_id", "wrong"),
        ("candidate_hash", "0" * 64),
        ("proposal_hash", "0" * 64),
        ("owner_receipts", []),
    ]
    for field, wrong in invalid:
        with pytest.raises(ValueError, match="gate_receipt_mismatch"):
            verify_gate(
                command,
                "map_manuscript",
                [draft],
                {**response, "receipts": [{**receipt, field: wrong}]},
            )
