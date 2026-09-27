"""Non-billable provider credential check on the Agent Temporal queue."""

import base64
import binascii
from typing import Any
from uuid import UUID

import httpx
from pydantic import BaseModel, ConfigDict, Field, ValidationError, field_validator
from temporalio import activity
from temporalio.exceptions import ApplicationError

from app.providers.credential_crypto import CredentialOpener, InvalidCredentialEnvelope

_OPENROUTER_CURRENT_KEY_URL = "https://openrouter.ai/api/v1/key"


class CredentialReference(BaseModel):
    model_config = ConfigDict(extra="forbid")

    id: UUID
    key_id: str = Field(min_length=1, max_length=128)
    ciphertext: str = Field(min_length=1, max_length=32768)

    @field_validator("id")
    @classmethod
    def nonzero_id(cls, value: UUID) -> UUID:
        if value.int == 0:
            raise ValueError("invalid credential identity")
        return value


class CredentialTestInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    provider_id: UUID
    provider_key: str = Field(min_length=1, max_length=128)
    adapter_key: str = Field(min_length=1, max_length=128)
    credential: CredentialReference

    @field_validator("provider_id")
    @classmethod
    def nonzero_provider_id(cls, value: UUID) -> UUID:
        if value.int == 0:
            raise ValueError("invalid provider identity")
        return value


class CredentialTestActivities:
    def __init__(self, opener: CredentialOpener | None, client: httpx.AsyncClient) -> None:
        self._opener = opener
        self._client = client

    @activity.defn(name="provider.test_credential")
    async def test_credential(self, payload: dict[str, Any]) -> dict[str, str]:
        try:
            data = CredentialTestInput.model_validate(payload)
            ciphertext = base64.b64decode(data.credential.ciphertext, validate=True)
            if base64.b64encode(ciphertext).decode("ascii") != data.credential.ciphertext:
                raise ValueError("non-canonical ciphertext encoding")
        except (ValidationError, ValueError, binascii.Error):
            raise ApplicationError(
                "invalid credential activity input",
                type="invalid_activity_input",
                non_retryable=True,
            ) from None

        # Only verified non-billable adapter probes may produce an OK result.
        if data.adapter_key != "openrouter":
            return {"result": "unsupported"}
        if self._opener is None:
            raise ApplicationError(
                "credential test unavailable",
                type="credential_test_unavailable",
                non_retryable=True,
            ) from None

        try:
            secret = self._opener.open(
                data.provider_id, data.credential.id, data.credential.key_id, ciphertext
            )
        except InvalidCredentialEnvelope:
            raise ApplicationError(
                "invalid credential envelope",
                type="invalid_credential_envelope",
                non_retryable=True,
            ) from None
        api_key = secret.get("api_key")
        if (
            not isinstance(api_key, str)
            or not 4 <= len(api_key) <= 4096
            or any(char < "!" or char > "~" for char in api_key)
        ):
            raise ApplicationError(
                "invalid credential envelope",
                type="invalid_credential_envelope",
                non_retryable=True,
            ) from None

        try:
            response = await self._client.get(
                _OPENROUTER_CURRENT_KEY_URL,
                headers={"Authorization": f"Bearer {api_key}"},
            )
        except httpx.TimeoutException:
            return {"result": "timeout"}
        except httpx.RequestError:
            return {"result": "unreachable"}
        if response.status_code in (401, 403):
            return {"result": "auth_failed"}
        if response.status_code != 200 or len(response.content) > 65536:
            return {"result": "unreachable"}
        try:
            body = response.json()
        except ValueError:
            return {"result": "unreachable"}
        return {
            "result": "ok"
            if isinstance(body, dict) and isinstance(body.get("data"), dict)
            else "unreachable"
        }
