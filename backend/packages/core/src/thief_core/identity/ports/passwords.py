from __future__ import annotations

from typing import Protocol, runtime_checkable


@runtime_checkable
class PasswordHasher(Protocol):
    def hash(self, value: str) -> str: ...

    def verify(self, encoded: str, value: str) -> bool: ...
