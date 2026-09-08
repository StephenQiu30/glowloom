"""Authenticated application HTTP boundaries; never imported by the model process."""

from __future__ import annotations

import asyncio
import base64
import hashlib
import hmac
import json
import time
from typing import Any

import httpx
from pydantic import TypeAdapter

from app.creation.authorization import HEADER
from app.creation.contract import Command, decode_object
from app.protocol.canonical import canonical_hash
from app.text_contract.authorization import sign_task
from app.text_contract.failure import HarnessFailed, InvocationFailure
from app.text_contract.source import SourceEdition
from app.text_contract.task import Stage, TextTask


class PlatformUnavailable(RuntimeError):
    pass


def sign_request(secret: str, audience: str, path: str, body: bytes) -> str:
    claims = {
        "audience": audience,
        "method": "POST",
        "path": path,
        "body_hash": hashlib.sha256(body).hexdigest(),
        "expires_at": int(time.time()) + 60,
    }
    encoded = base64.urlsafe_b64encode(json.dumps(claims, separators=(",", ":")).encode()).rstrip(
        b"="
    )
    signature = base64.urlsafe_b64encode(hmac.digest(secret.encode(), encoded, "sha256")).rstrip(
        b"="
    )
    return (encoded + b"." + signature).decode()


async def bounded_json(
    client: httpx.AsyncClient,
    url: str,
    body: bytes,
    headers: dict[str, str],
    deadline_seconds: int,
    *,
    task: TextTask | None = None,
) -> dict[str, Any]:
    # Streaming bounds the response even when Content-Length is absent or dishonest.
    async with (
        asyncio.timeout(deadline_seconds),
        client.stream(
            "POST",
            url,
            content=body,
            headers=headers,
            timeout=deadline_seconds,
            follow_redirects=False,
        ) as response,
    ):
        if response.status_code != 200 and (
            task is None or response.status_code not in {409, 422, 502}
        ):
            raise PlatformUnavailable(f"internal_http_{response.status_code}")
        raw = bytearray()
        async for chunk in response.aiter_bytes():
            if len(raw) + len(chunk) > 16000000:
                raise ValueError("internal_response_too_large")
            raw.extend(chunk)
        if response.status_code != 200:
            try:
                envelope = decode_object(bytes(raw))
                if set(envelope) != {"detail"}:
                    raise ValueError("invalid failure envelope")
                failure = InvocationFailure.model_validate(envelope["detail"])
                expected_status = (
                    502
                    if failure.state == "unknown"
                    else (409 if failure.code == "skill_release_unavailable" else 422)
                )
                if response.status_code != expected_status:
                    raise ValueError("failure status mismatch")
                if task is None or (
                    failure.invocation_id,
                    failure.input_hash,
                    failure.release_hash,
                ) != (
                    task.invocation_id,
                    canonical_hash(task.model_dump(mode="json")),
                    task.release_hash,
                ):
                    raise ValueError("failure binding mismatch")
            except ValueError:
                raise PlatformUnavailable(f"internal_http_{response.status_code}") from None
            raise HarnessFailed(failure)
        return decode_object(bytes(raw))


def verify_gate(
    command: Command, stage: Stage, drafts: list[dict[str, Any]], response: dict[str, Any]
) -> dict[str, Any]:
    if response.get("gate") != stage or response.get("status") not in {
        "pending",
        "accepted",
        "rejected",
    }:
        raise ValueError("gate_response_invalid")
    if response["status"] != "accepted":
        return response
    receipts = response.get("receipts")
    if not isinstance(receipts, list):
        raise ValueError("gate_receipt_mismatch")
    receipts = TypeAdapter(list[dict[str, Any]]).validate_python(receipts, strict=True)
    if len(receipts) != len(drafts):
        raise ValueError("gate_receipt_mismatch")
    indexed = {item.get("step_id"): item for item in receipts}
    if len(indexed) != len(drafts):
        raise ValueError("gate_receipt_mismatch")
    for draft in drafts:
        receipt = indexed.get(draft["step_id"], {})
        if (
            receipt.get("schema") != "creation-adoption-production"
            or receipt.get("run_id") != command.run_id
            or receipt.get("gate") != stage
            or receipt.get("proposal_revision") != 1
            or receipt.get("proposal_hash") != draft["result_hash"]
            or receipt.get("candidate_hash") != draft["candidate_hash"]
            or not receipt.get("decision_id")
            or not receipt.get("owner_receipts")
        ):
            raise ValueError("gate_receipt_mismatch")
    return response


class PlatformClient:
    def __init__(self, client: httpx.AsyncClient, url: str, secret: str) -> None:
        self.client, self.url, self.secret = client, url.rstrip("/"), secret

    async def _post(
        self, command: Command, suffix: str, payload: dict[str, Any], audience: str
    ) -> dict[str, Any]:
        path = f"/internal/creation/runs/{command.run_id}/{suffix}"
        body = json.dumps(payload, separators=(",", ":")).encode()
        try:
            return await bounded_json(
                self.client,
                self.url + path,
                body,
                {
                    HEADER: sign_request(self.secret, audience, path, body),
                    "Content-Type": "application/json",
                },
                30,
            )
        except (httpx.HTTPError, TimeoutError) as error:
            raise PlatformUnavailable("platform_unavailable") from error

    async def source(self, command: Command) -> SourceEdition:
        raw = await self._post(
            command,
            "source",
            {"payload_hash": command.payload_hash},
            "lanverse.creation.platform.source",
        )
        try:
            source = SourceEdition.model_validate(raw)
        except ValueError:
            # Temporal failure history must not contain the original manuscript in a
            # Pydantic diagnostic. The validated source lives only in the trusted store.
            raise ValueError("platform_source_invalid") from None
        if (
            source.revision_id != command.source.revision_id
            or source.content_hash != command.source.content_hash
        ):
            raise ValueError("platform_source_mismatch")
        return source

    async def gate(
        self, command: Command, stage: Stage, drafts: list[dict[str, Any]]
    ) -> dict[str, Any]:
        refs = [
            {key: item[key] for key in ("draft_id", "step_id", "candidate_hash", "result_hash")}
            for item in drafts
        ]
        response = await self._post(
            command,
            f"gates/{stage}/resolve",
            {"payload_hash": command.payload_hash, "drafts": refs},
            "lanverse.creation.platform",
        )
        return verify_gate(command, stage, refs, response)


class HarnessClient:
    def __init__(self, client: httpx.AsyncClient, url: str, secret: str) -> None:
        self.client, self.url, self.secret = client, url.rstrip("/"), secret

    async def invoke(self, task: TextTask) -> dict[str, Any]:
        token = sign_task(task, self.secret, int(time.time()) + 60)
        return await bounded_json(
            self.client,
            self.url + "/internal/text-storyboard/invocations",
            task.model_dump_json().encode(),
            {
                "X-Lanverse-Text-Authorization": token,
                "Content-Type": "application/json",
            },
            task.timeout_seconds + 10,
            task=task,
        )
