from __future__ import annotations

from datetime import datetime
from types import TracebackType
from typing import Protocol, Self, runtime_checkable
from uuid import UUID

from thief.identity.model import Invitation, Session, User


@runtime_checkable
class PasswordHasher(Protocol):
    def hash(self, value: str) -> str: ...

    def verify(self, encoded: str, value: str) -> bool: ...


class IdentityRepository(Protocol):
    def find_invitation(self, token_hash: str) -> Invitation | None: ...

    def email_exists(self, email: str) -> bool: ...

    def add_user(self, user: User) -> None: ...

    def find_user_by_email(self, email: str) -> User | None: ...

    def find_user(self, user_id: UUID) -> User | None: ...

    def add_session(self, session: Session) -> None: ...

    def find_session(self, token_hash: str) -> Session | None: ...

    def revoke_session(self, session_id: UUID, revoked_at: datetime) -> bool: ...

    def accept_invitation(
        self,
        invitation_id: UUID,
        accepted_at: datetime,
    ) -> bool: ...


class IdentityUnitOfWork(Protocol):
    @property
    def identities(self) -> IdentityRepository: ...

    def __enter__(self) -> Self: ...

    def __exit__(
        self,
        exception_type: type[BaseException] | None,
        exception: BaseException | None,
        traceback: TracebackType | None,
    ) -> None: ...

    def commit(self) -> None: ...

    def rollback(self) -> None: ...
