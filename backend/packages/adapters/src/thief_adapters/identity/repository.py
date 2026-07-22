from __future__ import annotations

from datetime import datetime
from typing import Any, cast
from uuid import UUID

from sqlalchemy import text
from sqlalchemy.engine import CursorResult
from sqlalchemy.orm import Session as OrmSession

from thief_core.identity import Invitation, Role, Session, User


class SqlAlchemyIdentityRepository:
    def __init__(self, session: OrmSession) -> None:
        self._session = session

    def find_invitation(self, token_hash: str) -> Invitation | None:
        row = (
            self._session.execute(
                text(
                    "SELECT id, email, token_hash, role, invited_by, created_at, "
                    "expires_at, accepted_at, revoked_at "
                    "FROM identity.invitations WHERE token_hash = :token_hash"
                ),
                {"token_hash": token_hash},
            )
            .mappings()
            .one_or_none()
        )
        if row is None:
            return None
        return Invitation(
            id=cast(UUID, row["id"]),
            email=str(row["email"]),
            token_hash=str(row["token_hash"]),
            role=Role(str(row["role"])),
            invited_by=cast(UUID, row["invited_by"]),
            created_at=cast(datetime, row["created_at"]),
            expires_at=cast(datetime, row["expires_at"]),
            accepted_at=cast(datetime | None, row["accepted_at"]),
            revoked_at=cast(datetime | None, row["revoked_at"]),
        )

    def email_exists(self, email: str) -> bool:
        return (
            self._session.execute(
                text("SELECT 1 FROM identity.users WHERE email = :email"),
                {"email": email},
            ).scalar_one_or_none()
            is not None
        )

    def find_user_by_email(self, email: str) -> User | None:
        row = (
            self._session.execute(
                text(
                    "SELECT id, email, password_hash, role, is_active, created_at "
                    "FROM identity.users WHERE email = :email"
                ),
                {"email": email},
            )
            .mappings()
            .one_or_none()
        )
        return _user_from_row(row)

    def find_user(self, user_id: UUID) -> User | None:
        row = (
            self._session.execute(
                text(
                    "SELECT id, email, password_hash, role, is_active, created_at "
                    "FROM identity.users WHERE id = :id"
                ),
                {"id": user_id},
            )
            .mappings()
            .one_or_none()
        )
        return _user_from_row(row)

    def add_user(self, user: User) -> None:
        self._session.execute(
            text(
                "INSERT INTO identity.users "
                "(id, email, password_hash, role, is_active, created_at) "
                "VALUES (:id, :email, :password_hash, :role, :is_active, :created_at)"
            ),
            {
                "id": user.id,
                "email": user.email,
                "password_hash": user.password_hash,
                "role": user.role.value,
                "is_active": user.is_active,
                "created_at": user.created_at,
            },
        )

    def add_session(self, session: Session) -> None:
        self._session.execute(
            text(
                "INSERT INTO identity.sessions "
                "(id, user_id, token_hash, csrf_token_hash, created_at, expires_at) "
                "VALUES (:id, :user_id, :token_hash, :csrf_token_hash, "
                ":created_at, :expires_at)"
            ),
            {
                "id": session.id,
                "user_id": session.user_id,
                "token_hash": session.token_hash,
                "csrf_token_hash": session.csrf_token_hash,
                "created_at": session.created_at,
                "expires_at": session.expires_at,
            },
        )

    def find_session(self, token_hash: str) -> Session | None:
        row = (
            self._session.execute(
                text(
                    "SELECT id, user_id, token_hash, csrf_token_hash, created_at, "
                    "expires_at, revoked_at FROM identity.sessions "
                    "WHERE token_hash = :token_hash"
                ),
                {"token_hash": token_hash},
            )
            .mappings()
            .one_or_none()
        )
        if row is None:
            return None
        return Session(
            id=cast(UUID, row["id"]),
            user_id=cast(UUID, row["user_id"]),
            token_hash=str(row["token_hash"]),
            csrf_token_hash=str(row["csrf_token_hash"]),
            created_at=cast(datetime, row["created_at"]),
            expires_at=cast(datetime, row["expires_at"]),
            revoked_at=cast(datetime | None, row["revoked_at"]),
        )

    def revoke_session(self, session_id: UUID, revoked_at: datetime) -> bool:
        result = cast(
            CursorResult[Any],
            self._session.execute(
                text(
                    "UPDATE identity.sessions SET revoked_at = :revoked_at "
                    "WHERE id = :id AND revoked_at IS NULL AND expires_at > :revoked_at"
                ),
                {"id": session_id, "revoked_at": revoked_at},
            ),
        )
        return result.rowcount == 1

    def accept_invitation(
        self,
        invitation_id: UUID,
        accepted_at: datetime,
    ) -> bool:
        result = cast(
            CursorResult[Any],
            self._session.execute(
                text(
                    "UPDATE identity.invitations SET accepted_at = :accepted_at "
                    "WHERE id = :id AND accepted_at IS NULL AND revoked_at IS NULL "
                    "AND created_at <= :accepted_at AND expires_at > :accepted_at"
                ),
                {"id": invitation_id, "accepted_at": accepted_at},
            ),
        )
        return result.rowcount == 1


def _user_from_row(row: Any) -> User | None:
    if row is None:
        return None
    return User(
        id=cast(UUID, row["id"]),
        email=str(row["email"]),
        password_hash=str(row["password_hash"]),
        role=Role(str(row["role"])),
        is_active=bool(row["is_active"]),
        created_at=cast(datetime, row["created_at"]),
    )
