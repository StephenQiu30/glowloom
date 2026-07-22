from __future__ import annotations

from argon2 import PasswordHasher
from argon2.exceptions import InvalidHashError, VerifyMismatchError


class Argon2PasswordHasher:
    def __init__(self) -> None:
        self._hasher = PasswordHasher()

    def hash(self, value: str) -> str:
        return self._hasher.hash(value)

    def verify(self, encoded: str, value: str) -> bool:
        try:
            return self._hasher.verify(encoded, value)
        except (InvalidHashError, VerifyMismatchError):
            return False
