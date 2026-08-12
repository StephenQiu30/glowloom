from dataclasses import dataclass, field
from typing import Protocol
from uuid import UUID

from pydantic import SecretStr


class CredentialStoreUnavailableError(RuntimeError):
    """Raised when credential encryption keys cannot be used safely."""


class CredentialIntegrityError(RuntimeError):
    """Raised when encrypted credential identity or integrity is invalid."""


@dataclass(frozen=True, slots=True)
class CredentialIdentity:
    workspace_id: UUID
    connection_id: UUID
    credential_version_id: UUID
    version: int


@dataclass(frozen=True, slots=True)
class EncryptedCredential:
    key_id: str
    nonce: bytes = field(repr=False)
    ciphertext: bytes = field(repr=False)
    auth_tag: bytes = field(repr=False)
    fingerprint_hmac: str = field(repr=False)


class CredentialCipherPort(Protocol):
    def encrypt(
        self,
        *,
        identity: CredentialIdentity,
        credential: SecretStr,
    ) -> EncryptedCredential: ...

    def decrypt(
        self,
        *,
        identity: CredentialIdentity,
        encrypted: EncryptedCredential,
    ) -> SecretStr: ...
