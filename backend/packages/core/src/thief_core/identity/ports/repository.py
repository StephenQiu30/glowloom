from __future__ import annotations

from datetime import datetime
from typing import Protocol
from uuid import UUID

from thief_core.identity.domain import Invitation, Session, User
from thief_core.shared import UnitOfWork


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


class IdentityUnitOfWork(UnitOfWork, Protocol):
    @property
    def identities(self) -> IdentityRepository: ...
