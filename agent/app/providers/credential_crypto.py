"""Open the provider credential envelope produced by the Go backend."""

import json
from typing import cast
from uuid import UUID

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import padding, rsa
from cryptography.hazmat.primitives.ciphers.aead import AESGCM


class InvalidCredentialEnvelope(ValueError):
    """The credential cannot be opened with this identity and Agent key."""


class CredentialOpener:
    """Keep the Agent private key out of the backend and provider adapters."""

    def __init__(self, key_id: str, private_key_pem: bytes) -> None:
        if not key_id.strip() or len(key_id) > 128:
            raise ValueError("invalid credential key")
        try:
            private_key = serialization.load_pem_private_key(private_key_pem, password=None)
        except (TypeError, ValueError):
            raise ValueError("invalid credential key") from None
        if not isinstance(private_key, rsa.RSAPrivateKey) or private_key.key_size < 2048:
            raise ValueError("invalid credential key")
        self._key_id = key_id
        self._private_key = private_key

    def open(
        self, provider_id: UUID, credential_id: UUID, key_id: str, envelope: bytes
    ) -> dict[str, object]:
        """Authenticate and decode one JSON secret without exposing failure details."""
        wrapped_length = self._private_key.key_size // 8
        if (
            provider_id.int == 0
            or credential_id.int == 0
            or key_id != self._key_id
            or len(envelope) < wrapped_length + 12 + 16
        ):
            raise InvalidCredentialEnvelope("invalid credential envelope")
        wrapped_dek = envelope[:wrapped_length]
        nonce = envelope[wrapped_length : wrapped_length + 12]
        ciphertext = envelope[wrapped_length + 12 :]
        try:
            dek = self._private_key.decrypt(
                wrapped_dek,
                padding.OAEP(
                    mgf=padding.MGF1(algorithm=hashes.SHA256()),
                    algorithm=hashes.SHA256(),
                    label=None,
                ),
            )
            if len(dek) != 32:
                raise InvalidCredentialEnvelope("invalid credential envelope")
            plaintext = AESGCM(dek).decrypt(
                nonce, ciphertext, provider_id.bytes + credential_id.bytes
            )
            secret = json.loads(plaintext)
        except (InvalidTag, ValueError):
            raise InvalidCredentialEnvelope("invalid credential envelope") from None
        if not isinstance(secret, dict) or not secret:
            raise InvalidCredentialEnvelope("invalid credential envelope")
        return cast("dict[str, object]", secret)
