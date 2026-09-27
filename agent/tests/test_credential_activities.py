import asyncio
import base64
import json
import os
from datetime import timedelta
from pathlib import Path
from typing import Any, cast
from uuid import UUID, uuid4

import httpx
import pytest
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import padding, rsa
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from temporalio import workflow
from temporalio.client import Client
from temporalio.exceptions import ApplicationError
from temporalio.testing import ActivityEnvironment
from temporalio.worker import Worker

from app.harness.mock_model import MockStructuredClient
from app.harness.router import ModelRouter, Price
from app.harness.skills import SkillRegistry
from app.providers.credential_crypto import CredentialOpener
from app.worker.credential_activities import CredentialTestActivities
from app.worker.skill_activities import create_skill_worker

PROVIDER_ID = UUID("49f48d62-ff66-44c5-a25d-ea131d4a607d")
CREDENTIAL_ID = UUID("8ff0a79a-0e57-4754-a0e9-93fab15409fa")
TEST_KEY = "local-test-api-key"


def _activity_input(adapter_key: str = "openrouter") -> tuple[CredentialOpener, dict[str, object]]:
    private_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    private_pem = private_key.private_bytes(
        serialization.Encoding.PEM,
        serialization.PrivateFormat.PKCS8,
        serialization.NoEncryption(),
    )
    dek = AESGCM.generate_key(bit_length=256)
    wrapped = private_key.public_key().encrypt(
        dek,
        padding.OAEP(mgf=padding.MGF1(hashes.SHA256()), algorithm=hashes.SHA256(), label=None),
    )
    nonce = bytes(range(12))
    ciphertext = AESGCM(dek).encrypt(
        nonce,
        json.dumps({"api_key": TEST_KEY}).encode(),
        PROVIDER_ID.bytes + CREDENTIAL_ID.bytes,
    )
    return CredentialOpener("agent-test", private_pem), {
        "provider_id": str(PROVIDER_ID),
        "provider_key": "configured-provider",
        "adapter_key": adapter_key,
        "credential": {
            "id": str(CREDENTIAL_ID),
            "key_id": "agent-test",
            "ciphertext": base64.b64encode(wrapped + nonce + ciphertext).decode(),
        },
    }


def test_openrouter_uses_only_free_current_key_request() -> None:
    opener, payload = _activity_input()
    requests: list[httpx.Request] = []

    def respond(request: httpx.Request) -> httpx.Response:
        requests.append(request)
        return httpx.Response(200, json={"data": {"label": "safe-summary"}})

    async def run() -> None:
        async with httpx.AsyncClient(transport=httpx.MockTransport(respond)) as client:
            result = await ActivityEnvironment().run(
                CredentialTestActivities(opener, client).test_credential, payload
            )
        assert result == {"result": "ok"}

    asyncio.run(run())
    assert len(requests) == 1
    assert requests[0].method == "GET"
    assert str(requests[0].url) == "https://openrouter.ai/api/v1/key"
    assert requests[0].content == b""
    assert requests[0].headers["Authorization"] == f"Bearer {TEST_KEY}"


@pytest.mark.parametrize(
    ("status", "body", "expected"),
    [
        (401, b"credential leaked in vendor error", "auth_failed"),
        (403, b"credential leaked in vendor error", "auth_failed"),
        (302, b"", "unreachable"),
        (500, b"credential leaked in vendor error", "unreachable"),
        (200, b"not-json", "unreachable"),
    ],
)
def test_probe_only_returns_safe_result(status: int, body: bytes, expected: str) -> None:
    opener, payload = _activity_input()

    async def run() -> None:
        async with httpx.AsyncClient(
            transport=httpx.MockTransport(lambda _: httpx.Response(status, content=body))
        ) as client:
            result = await ActivityEnvironment().run(
                CredentialTestActivities(opener, client).test_credential, payload
            )
        assert result == {"result": expected}
        assert TEST_KEY not in str(result)
        assert "credential leaked" not in str(result)

    asyncio.run(run())


def test_timeout_and_network_failure_do_not_expose_exception() -> None:
    opener, payload = _activity_input()

    async def timeout(_: httpx.Request) -> httpx.Response:
        raise httpx.ReadTimeout("vendor returned sensitive text")

    async def network(_: httpx.Request) -> httpx.Response:
        raise httpx.ConnectError("vendor returned sensitive text")

    async def run() -> None:
        for callback, expected in [(timeout, "timeout"), (network, "unreachable")]:
            async with httpx.AsyncClient(transport=httpx.MockTransport(callback)) as client:
                result = await ActivityEnvironment().run(
                    CredentialTestActivities(opener, client).test_credential, payload
                )
            assert result == {"result": expected}

    asyncio.run(run())


def test_unsupported_adapter_never_sends_request() -> None:
    opener, payload = _activity_input("minimax")

    def unexpected(_: httpx.Request) -> httpx.Response:
        pytest.fail("unsupported adapter sent a request")

    async def run() -> None:
        async with httpx.AsyncClient(transport=httpx.MockTransport(unexpected)) as client:
            result = await ActivityEnvironment().run(
                CredentialTestActivities(opener, client).test_credential, payload
            )
        assert result == {"result": "unsupported"}

    asyncio.run(run())


def test_missing_agent_private_key_returns_unavailable_without_egress() -> None:
    _, payload = _activity_input()

    def unexpected(_: httpx.Request) -> httpx.Response:
        pytest.fail("unconfigured Agent sent a request")

    async def run() -> None:
        async with httpx.AsyncClient(transport=httpx.MockTransport(unexpected)) as client:
            with pytest.raises(ApplicationError) as raised:
                await ActivityEnvironment().run(
                    CredentialTestActivities(None, client).test_credential, payload
                )
        assert raised.value.type == "credential_test_unavailable"
        assert raised.value.non_retryable

    asyncio.run(run())


def test_wrong_identity_and_malformed_payload_are_generic_failures() -> None:
    opener, payload = _activity_input()
    credential = payload["credential"]
    assert isinstance(credential, dict)

    def unexpected(_: httpx.Request) -> httpx.Response:
        pytest.fail("invalid credential sent a request")

    async def run() -> None:
        async with httpx.AsyncClient(transport=httpx.MockTransport(unexpected)) as client:
            activities = CredentialTestActivities(opener, client)
            cases: list[tuple[dict[str, object], str]] = [
                ({"provider_id": str(UUID(int=1))}, "invalid_credential_envelope"),
                (
                    {"credential": {**credential, "ciphertext": "invalid!"}},
                    "invalid_activity_input",
                ),
                ({"provider_id": "not-uuid"}, "invalid_activity_input"),
                ({"secret": TEST_KEY}, "invalid_activity_input"),
            ]
            for override, expected in cases:
                with pytest.raises(ApplicationError) as raised:
                    await ActivityEnvironment().run(
                        activities.test_credential, {**payload, **override}
                    )
                assert raised.value.type == expected
                assert raised.value.non_retryable
                assert TEST_KEY not in str(raised.value)

    asyncio.run(run())


@workflow.defn(sandboxed=False)
class CredentialActivityProbe:
    @workflow.run
    async def run(self, queue: str, payload: dict[str, Any]) -> dict[str, str]:
        result = await workflow.execute_activity(
            "provider.test_credential",
            payload,
            task_queue=queue,
            start_to_close_timeout=timedelta(seconds=10),
        )
        return cast("dict[str, str]", result)


def test_local_temporal_routes_credential_test_on_agent_queue() -> None:
    temporal_addr = os.getenv("LV_TEST_TEMPORAL_ADDR")
    namespace = os.getenv("LV_TEST_TEMPORAL_NAMESPACE")
    if not temporal_addr or not namespace:
        pytest.skip("local Temporal test address and namespace are not configured")
    opener, payload = _activity_input()
    registry = SkillRegistry.load(Path(__file__).resolve().parents[1] / "skills")
    router = ModelRouter(MockStructuredClient(), {"mock.structured": Price(0, 0)})

    async def run() -> None:
        client = await Client.connect(temporal_addr, namespace=namespace)
        token = uuid4().hex
        agent_queue = f"lanverse-credential-agent-{token}"
        flow_queue = f"lanverse-credential-flow-{token}"

        def respond(request: httpx.Request) -> httpx.Response:
            assert str(request.url) == "https://openrouter.ai/api/v1/key"
            return httpx.Response(200, json={"data": {"label": "test"}})

        async with (
            httpx.AsyncClient(transport=httpx.MockTransport(respond)) as http_client,
            create_skill_worker(
                client,
                registry,
                router,
                task_queue=agent_queue,
                credential_tests=CredentialTestActivities(opener, http_client),
            ),
            Worker(
                client, task_queue=flow_queue, workflows=[CredentialActivityProbe], activities=[]
            ),
        ):
            result = await asyncio.wait_for(
                client.execute_workflow(
                    CredentialActivityProbe.run,
                    args=[agent_queue, payload],
                    id=f"lanverse-credential-test-{token}",
                    task_queue=flow_queue,
                ),
                timeout=20,
            )
        assert result == {"result": "ok"}

    asyncio.run(run())
