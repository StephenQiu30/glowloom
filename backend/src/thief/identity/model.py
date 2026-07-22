from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from uuid import UUID


class Role(StrEnum):
    CREATOR = "creator"
    ADMIN = "admin"


def normalize_email(value: str) -> str:
    normalized = value.strip().lower()
    if not normalized:
        raise ValueError("email is required")
    if len(normalized) > 320:
        raise ValueError("email is too long")
    return normalized


@dataclass(frozen=True, slots=True)
class Principal:
    user_id: UUID
    role: Role

    def has_role(self, required: Role) -> bool:
        return self.role is required

    def owns(self, owner_id: UUID) -> bool:
        return self.user_id == owner_id


@dataclass(frozen=True, slots=True)
class User:
    id: UUID
    email: str
    password_hash: str
    role: Role
    created_at: datetime
    is_active: bool = True


@dataclass(frozen=True, slots=True)
class Invitation:
    id: UUID
    email: str
    role: Role
    token_hash: str
    invited_by: UUID
    created_at: datetime
    expires_at: datetime
    accepted_at: datetime | None = None
    revoked_at: datetime | None = None

    def is_usable_at(self, now: datetime) -> bool:
        return (
            self.accepted_at is None
            and self.revoked_at is None
            and self.created_at <= now < self.expires_at
        )


@dataclass(frozen=True, slots=True)
class Session:
    id: UUID
    user_id: UUID
    token_hash: str
    csrf_token_hash: str
    created_at: datetime
    expires_at: datetime
    revoked_at: datetime | None = None

    def is_active_at(self, now: datetime) -> bool:
        return self.revoked_at is None and self.created_at <= now < self.expires_at
