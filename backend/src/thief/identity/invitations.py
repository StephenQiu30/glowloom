from __future__ import annotations

import hashlib
from collections.abc import Callable
from dataclasses import dataclass
from datetime import datetime
from uuid import UUID

from thief.identity.model import Role, User
from thief.identity.ports import IdentityUnitOfWork, PasswordHasher


class InvitationNotUsable(Exception):
    pass


class EmailAlreadyRegistered(Exception):
    pass


@dataclass(frozen=True, slots=True)
class AcceptInvitationCommand:
    token: str
    password: str


@dataclass(frozen=True, slots=True)
class AcceptInvitationResult:
    user_id: UUID
    email: str
    role: Role


class AcceptInvitation:
    def __init__(
        self,
        *,
        unit_of_work: Callable[[], IdentityUnitOfWork],
        passwords: PasswordHasher,
        now: Callable[[], datetime],
        new_id: Callable[[], UUID],
    ) -> None:
        self._unit_of_work = unit_of_work
        self._passwords = passwords
        self._now = now
        self._new_id = new_id

    def execute(self, command: AcceptInvitationCommand) -> AcceptInvitationResult:
        if not command.token or not command.password:
            raise InvitationNotUsable

        current_time = self._now()
        token_hash = hashlib.sha256(command.token.encode()).hexdigest()
        with self._unit_of_work() as unit_of_work:
            invitation = unit_of_work.identities.find_invitation(token_hash)
            if invitation is None or not invitation.is_usable_at(current_time):
                raise InvitationNotUsable
            if unit_of_work.identities.email_exists(invitation.email):
                raise EmailAlreadyRegistered

            user = User(
                id=self._new_id(),
                email=invitation.email,
                password_hash=self._passwords.hash(command.password),
                role=invitation.role,
                created_at=current_time,
            )
            unit_of_work.identities.add_user(user)
            if not unit_of_work.identities.accept_invitation(
                invitation.id,
                current_time,
            ):
                raise InvitationNotUsable
            unit_of_work.commit()

        return AcceptInvitationResult(user.id, user.email, user.role)
