import base64
import json
import os
from uuid import UUID

import pytest
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import padding, rsa
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

from app.providers.credential_crypto import CredentialOpener, InvalidCredentialEnvelope

PROVIDER_ID = UUID("49f48d62-ff66-44c5-a25d-ea131d4a607d")
CREDENTIAL_ID = UUID("8ff0a79a-0e57-4754-a0e9-93fab15409fa")


def _key_pair() -> tuple[rsa.RSAPrivateKey, bytes]:
    private_key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    private_pem = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    )
    return private_key, private_pem


def _envelope(private_key: rsa.RSAPrivateKey) -> bytes:
    dek = AESGCM.generate_key(bit_length=256)
    wrapped_dek = private_key.public_key().encrypt(
        dek,
        padding.OAEP(mgf=padding.MGF1(hashes.SHA256()), algorithm=hashes.SHA256(), label=None),
    )
    nonce = bytes(range(12))
    ciphertext = AESGCM(dek).encrypt(
        nonce,
        json.dumps({"api_key": "local-test-value"}).encode(),
        PROVIDER_ID.bytes + CREDENTIAL_ID.bytes,
    )
    return wrapped_dek + nonce + ciphertext


def test_opens_documented_envelope() -> None:
    private_key, private_pem = _key_pair()
    opener = CredentialOpener("agent-2026", private_pem)

    assert opener.open(PROVIDER_ID, CREDENTIAL_ID, "agent-2026", _envelope(private_key)) == {
        "api_key": "local-test-value"
    }


def test_rejects_wrong_identity_key_and_modified_envelope() -> None:
    private_key, private_pem = _key_pair()
    opener = CredentialOpener("agent-2026", private_pem)
    envelope = _envelope(private_key)
    for provider_id, credential_id, key_id, payload in [
        (UUID(int=0), CREDENTIAL_ID, "agent-2026", envelope),
        (PROVIDER_ID, UUID(int=0), "agent-2026", envelope),
        (UUID(int=1), CREDENTIAL_ID, "agent-2026", envelope),
        (PROVIDER_ID, UUID(int=1), "agent-2026", envelope),
        (PROVIDER_ID, CREDENTIAL_ID, "other-key", envelope),
        (PROVIDER_ID, CREDENTIAL_ID, "agent-2026", envelope[:-1]),
        (PROVIDER_ID, CREDENTIAL_ID, "agent-2026", envelope[:-1] + bytes([envelope[-1] ^ 1])),
    ]:
        with pytest.raises(InvalidCredentialEnvelope):
            opener.open(provider_id, credential_id, key_id, payload)
    _, wrong_private_pem = _key_pair()
    with pytest.raises(InvalidCredentialEnvelope):
        CredentialOpener("agent-2026", wrong_private_pem).open(
            PROVIDER_ID, CREDENTIAL_ID, "agent-2026", envelope
        )


def test_rejects_invalid_private_key() -> None:
    with pytest.raises(ValueError, match="invalid credential key"):
        CredentialOpener("agent-2026", b"not a PEM key")
    weak_key = rsa.generate_private_key(public_exponent=65537, key_size=1024)  # noqa: S505 - rejection case
    weak_pem = weak_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption(),
    )
    with pytest.raises(ValueError, match="invalid credential key"):
        CredentialOpener("agent-2026", weak_pem)
    with pytest.raises(ValueError, match="invalid credential key"):
        CredentialOpener("   ", weak_pem)


def test_go_envelope_interoperability() -> None:
    encoded = os.environ.get("LV_TEST_CREDENTIAL_INTEROP_PAYLOAD")
    if encoded is None:
        pytest.skip("Go interoperability payload was not supplied")
    data = json.loads(base64.b64decode(encoded))
    opener = CredentialOpener(data["key_id"], base64.b64decode(data["private_key"]))
    assert opener.open(
        UUID(data["provider_id"]),
        UUID(data["credential_id"]),
        data["key_id"],
        base64.b64decode(data["ciphertext"]),
    ) == {"api_key": "local-test-value"}
