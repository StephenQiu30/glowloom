from __future__ import annotations

import hashlib
import hmac
from collections.abc import Callable
from dataclasses import dataclass
from datetime import datetime, timedelta
from uuid import UUID

from thief.identity.model import Role, Session, normalize_email
from thief.identity.ports import IdentityUnitOfWork, PasswordHasher


class InvalidCredentials(Exception):
    pass


class InvalidSession(Exception):
    pass


class InvalidCsrf(Exception):
    pass


@dataclass(frozen=True, slots=True)
class SessionIdentity:
    user_id: UUID
    email: str
    role: Role


@dataclass(frozen=True, slots=True)
class CreatedSession(SessionIdentity):
    session_token: str
    csrf_token: str


class IdentitySessions:
    def __init__(
        self,
        *,
        unit_of_work: Callable[[], IdentityUnitOfWork],
        passwords: PasswordHasher,
        now: Callable[[], datetime],
        new_id: Callable[[], UUID],
        new_secret: Callable[[], str],
        session_ttl: timedelta,
    ) -> None:
        self._unit_of_work = unit_of_work
        self._passwords = passwords
        self._now = now
        self._new_id = new_id
        self._new_secret = new_secret
        self._session_ttl = session_ttl

    def create(self, email: str, password: str) -> CreatedSession:
        normalized_email = normalize_email(email)
        with self._unit_of_work() as unit_of_work:
            user = unit_of_work.identities.find_user_by_email(normalized_email)
            if (
                user is None
                or not user.is_active
                or not self._passwords.verify(user.password_hash, password)
            ):
                raise InvalidCredentials

            current_time = self._now()
            session_token = self._new_secret()
            csrf_token = self._new_secret()
            unit_of_work.identities.add_session(
                Session(
                    id=self._new_id(),
                    user_id=user.id,
                    token_hash=_hash_secret(session_token),
                    csrf_token_hash=_hash_secret(csrf_token),
                    created_at=current_time,
                    expires_at=current_time + self._session_ttl,
                )
            )
            unit_of_work.commit()

        return CreatedSession(
            user.id,
            user.email,
            user.role,
            session_token,
            csrf_token,
        )

    def resolve(self, session_token: str) -> SessionIdentity:
        current_time = self._now()
        with self._unit_of_work() as unit_of_work:
            session = unit_of_work.identities.find_session(
                _hash_secret(session_token)
            )
            if session is None or not session.is_active_at(current_time):
                raise InvalidSession
            user = unit_of_work.identities.find_user(session.user_id)
            if user is None or not user.is_active:
                raise InvalidSession

        return SessionIdentity(user.id, user.email, user.role)

    def revoke(self, session_token: str, csrf_token: str) -> None:
        current_time = self._now()
        with self._unit_of_work() as unit_of_work:
            session = unit_of_work.identities.find_session(
                _hash_secret(session_token)
            )
            if session is None or not session.is_active_at(current_time):
                raise InvalidSession
            if not hmac.compare_digest(
                session.csrf_token_hash,
                _hash_secret(csrf_token),
            ):
                raise InvalidCsrf
            if not unit_of_work.identities.revoke_session(
                session.id,
                current_time,
            ):
                raise InvalidSession
            unit_of_work.commit()


def _hash_secret(value: str) -> str:
    return hashlib.sha256(value.encode()).hexdigest()
