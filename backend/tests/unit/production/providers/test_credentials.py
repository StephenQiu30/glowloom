from base64 import urlsafe_b64encode
from dataclasses import replace
from uuid import UUID

import pytest
from pydantic import SecretStr

from app.core.config import Settings
from app.modules.production.providers.contracts import (
    CredentialIdentity,
    CredentialIntegrityError,
    CredentialStoreUnavailableError,
)
from app.modules.production.providers.credentials import (
    AesGcmCredentialCipher,
    build_credential_cipher,
)

IDENTITY = CredentialIdentity(
    workspace_id=UUID("0191f6f4-0e8b-7a31-a630-95e8c36554fd"),
    connection_id=UUID("0191f6f4-2ec2-788e-8d40-6b934fc6bd68"),
    credential_version_id=UUID("0191f6f4-3af9-786a-b068-7e72fdbb9622"),
    version=1,
)


def _cipher() -> AesGcmCredentialCipher:
    return AesGcmCredentialCipher(
        key_id="provider-key-v1",
        master_key=b"m" * 32,
        fingerprint_key=b"f" * 32,
    )


def test_credential_cipher_round_trips_without_exposing_plaintext() -> None:
    plaintext = "sentinel-provider-credential-round-trip"
    cipher = _cipher()

    first = cipher.encrypt(identity=IDENTITY, credential=SecretStr(plaintext))
    second = cipher.encrypt(identity=IDENTITY, credential=SecretStr(plaintext))

    assert cipher.decrypt(identity=IDENTITY, encrypted=first).get_secret_value() == plaintext
    assert len(first.nonce) == 12
    assert len(first.auth_tag) == 16
    assert len(first.fingerprint_hmac) == 64
    assert first.nonce != second.nonce
    assert first.ciphertext != second.ciphertext
    assert first.fingerprint_hmac == second.fingerprint_hmac
    assert plaintext.encode() not in first.nonce + first.ciphertext + first.auth_tag
    assert plaintext not in repr(first)


@pytest.mark.parametrize(
    "tampered",
    (
        replace(
            IDENTITY,
            workspace_id=UUID("0191f6f5-82b0-763c-8c88-a1a1bdef8303"),
        ),
        replace(
            IDENTITY,
            connection_id=UUID("0191f6f5-9bb6-7867-9f57-1ff69678d475"),
        ),
        replace(
            IDENTITY,
            credential_version_id=UUID("0191f6f5-a7c0-7066-8dd9-a0c43af18e73"),
        ),
        replace(IDENTITY, version=2),
    ),
)
def test_credential_cipher_rejects_aad_identity_replay(
    tampered: CredentialIdentity,
) -> None:
    cipher = _cipher()
    encrypted = cipher.encrypt(identity=IDENTITY, credential=SecretStr("test-only-secret"))

    with pytest.raises(CredentialIntegrityError, match="integrity"):
        cipher.decrypt(identity=tampered, encrypted=encrypted)


@pytest.mark.parametrize("field", ("ciphertext", "auth_tag"))
def test_credential_cipher_rejects_bit_flips(field: str) -> None:
    cipher = _cipher()
    encrypted = cipher.encrypt(identity=IDENTITY, credential=SecretStr("test-only-secret"))
    original = getattr(encrypted, field)
    mutated = bytes((original[0] ^ 1,)) + original[1:]

    with pytest.raises(CredentialIntegrityError, match="integrity"):
        cipher.decrypt(
            identity=IDENTITY,
            encrypted=replace(encrypted, **{field: mutated}),
        )


def test_credential_cipher_fails_closed_for_unknown_key_id() -> None:
    cipher = _cipher()
    encrypted = cipher.encrypt(identity=IDENTITY, credential=SecretStr("test-only-secret"))

    with pytest.raises(CredentialStoreUnavailableError, match="unavailable"):
        cipher.decrypt(
            identity=IDENTITY,
            encrypted=replace(encrypted, key_id="retired-provider-key"),
        )


def test_credential_cipher_errors_do_not_expose_submitted_value() -> None:
    plaintext = "sentinel-provider-credential-error-output"
    cipher = _cipher()
    encrypted = cipher.encrypt(identity=IDENTITY, credential=SecretStr(plaintext))

    with pytest.raises(CredentialIntegrityError) as captured:
        cipher.decrypt(
            identity=replace(IDENTITY, version=2),
            encrypted=encrypted,
        )

    assert plaintext not in str(captured.value)
    assert plaintext not in repr(captured.value)


def test_credential_cipher_factory_requires_two_independent_32_byte_keys() -> None:
    with pytest.raises(CredentialStoreUnavailableError, match="unavailable"):
        build_credential_cipher(Settings())

    with pytest.raises(CredentialStoreUnavailableError, match="unavailable"):
        build_credential_cipher(
            Settings(
                provider_credential_master_key=SecretStr("not-base64"),
                provider_credential_fingerprint_key=SecretStr("also-not-base64"),
            )
        )

    configured = Settings(
        provider_credential_key_id="provider-key-v1",
        provider_credential_master_key=SecretStr(
            urlsafe_b64encode(b"m" * 32).decode("ascii")
        ),
        provider_credential_fingerprint_key=SecretStr(
            urlsafe_b64encode(b"f" * 32).decode("ascii")
        ),
    )

    encrypted = build_credential_cipher(configured).encrypt(
        identity=IDENTITY,
        credential=SecretStr("configured-test-secret"),
    )
    assert encrypted.key_id == "provider-key-v1"
