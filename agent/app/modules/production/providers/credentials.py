import hmac
import os
import re
from base64 import b64decode
from binascii import Error as Base64Error
from hashlib import sha256

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from pydantic import SecretStr

from app.core.config import Settings
from app.modules.production.providers.contracts import (
    CredentialIdentity,
    CredentialIntegrityError,
    CredentialStoreUnavailableError,
    EncryptedCredential,
)

_AES_256_KEY_BYTES = 32
_GCM_NONCE_BYTES = 12
_GCM_TAG_BYTES = 16
_URLSAFE_BASE64_32_BYTES = re.compile(rb"[A-Za-z0-9_-]{43}=")


def _identity_aad(identity: CredentialIdentity) -> bytes:
    if identity.version < 1:
        raise CredentialIntegrityError("provider credential integrity check failed")
    return (
        "lanverse-provider-credential:v1:"
        f"{identity.workspace_id}:{identity.connection_id}:"
        f"{identity.credential_version_id}:{identity.version}"
    ).encode("ascii")


def _decode_deployment_key(secret: SecretStr | None) -> bytes:
    if secret is None:
        raise CredentialStoreUnavailableError("provider credential store unavailable")
    try:
        encoded = secret.get_secret_value().encode("ascii")
        if _URLSAFE_BASE64_32_BYTES.fullmatch(encoded) is None:
            raise ValueError("invalid deployment key encoding")
        decoded = b64decode(encoded, altchars=b"-_", validate=True)
    except (UnicodeEncodeError, Base64Error, ValueError) as error:
        raise CredentialStoreUnavailableError("provider credential store unavailable") from error
    if len(decoded) != _AES_256_KEY_BYTES:
        raise CredentialStoreUnavailableError("provider credential store unavailable")
    return decoded


class AesGcmCredentialCipher:
    def __init__(
        self,
        *,
        key_id: str,
        master_key: bytes,
        fingerprint_key: bytes,
    ) -> None:
        if not key_id.strip():
            raise CredentialStoreUnavailableError("provider credential store unavailable")
        if len(master_key) != _AES_256_KEY_BYTES:
            raise CredentialStoreUnavailableError("provider credential store unavailable")
        if len(fingerprint_key) != _AES_256_KEY_BYTES:
            raise CredentialStoreUnavailableError("provider credential store unavailable")
        if hmac.compare_digest(master_key, fingerprint_key):
            raise CredentialStoreUnavailableError("provider credential store unavailable")
        self._key_id = key_id
        self._master_key = master_key
        self._fingerprint_key = fingerprint_key

    def encrypt(
        self,
        *,
        identity: CredentialIdentity,
        credential: SecretStr,
    ) -> EncryptedCredential:
        plaintext = credential.get_secret_value().encode("utf-8")
        if not plaintext:
            raise CredentialIntegrityError("provider credential must not be empty")
        nonce = os.urandom(_GCM_NONCE_BYTES)
        combined = AESGCM(self._master_key).encrypt(
            nonce,
            plaintext,
            _identity_aad(identity),
        )
        return EncryptedCredential(
            key_id=self._key_id,
            nonce=nonce,
            ciphertext=combined[:-_GCM_TAG_BYTES],
            auth_tag=combined[-_GCM_TAG_BYTES:],
            fingerprint_hmac=hmac.new(
                self._fingerprint_key,
                plaintext,
                sha256,
            ).hexdigest(),
        )

    def decrypt(
        self,
        *,
        identity: CredentialIdentity,
        encrypted: EncryptedCredential,
    ) -> SecretStr:
        if encrypted.key_id != self._key_id:
            raise CredentialStoreUnavailableError("provider credential store unavailable")
        if len(encrypted.nonce) != _GCM_NONCE_BYTES or len(encrypted.auth_tag) != _GCM_TAG_BYTES:
            raise CredentialIntegrityError("provider credential integrity check failed")
        try:
            plaintext = AESGCM(self._master_key).decrypt(
                encrypted.nonce,
                encrypted.ciphertext + encrypted.auth_tag,
                _identity_aad(identity),
            )
            return SecretStr(plaintext.decode("utf-8"))
        except (InvalidTag, UnicodeDecodeError, ValueError) as error:
            raise CredentialIntegrityError("provider credential integrity check failed") from error


def build_credential_cipher(settings: Settings) -> AesGcmCredentialCipher:
    return AesGcmCredentialCipher(
        key_id=settings.provider_credential_key_id,
        master_key=_decode_deployment_key(settings.provider_credential_master_key),
        fingerprint_key=_decode_deployment_key(settings.provider_credential_fingerprint_key),
    )
