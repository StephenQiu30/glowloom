from __future__ import annotations

import hashlib
import sys
import unittest
from datetime import UTC, datetime, timedelta
from pathlib import Path
from uuid import UUID, uuid4


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "src"))


class FakePasswords:
    def verify(self, encoded: str, value: str) -> bool:
        return (encoded, value) == ("password-hash", "safe password")

    def hash(self, value: str) -> str:
        return f"hashed:{value}"


class FakeSessionsRepository:
    def __init__(self, user: object, session: object | None = None) -> None:
        self.user = user
        self.session = session
        self.added_session: object | None = None
        self.revoked_id: UUID | None = None

    def find_user_by_email(self, email: str) -> object | None:
        return self.user if email == getattr(self.user, "email") else None

    def find_user(self, user_id: UUID) -> object | None:
        return self.user if user_id == getattr(self.user, "id") else None

    def add_session(self, session: object) -> None:
        self.added_session = session

    def find_session(self, token_hash: str) -> object | None:
        expected = getattr(self.session, "token_hash", None)
        return self.session if token_hash == expected else None

    def revoke_session(self, session_id: UUID, revoked_at: datetime) -> bool:
        self.revoked_id = session_id
        return True


class FakeIdentityUnitOfWork:
    def __init__(self, repository: FakeSessionsRepository) -> None:
        self.identities = repository
        self.commits = 0

    def __enter__(self) -> FakeIdentityUnitOfWork:
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def commit(self) -> None:
        self.commits += 1

    def rollback(self) -> None:
        return None


class SessionUseCaseTests(unittest.TestCase):
    def setUp(self) -> None:
        from thief.identity.model import Role, User

        self.now = datetime(2026, 7, 22, 10, 0, tzinfo=UTC)
        self.user = User(
            id=uuid4(),
            email="creator@example.com",
            password_hash="password-hash",
            role=Role.CREATOR,
            created_at=self.now,
        )

    def test_create_stores_only_session_and_csrf_hashes(self) -> None:
        from thief.identity.sessions import IdentitySessions

        repository = FakeSessionsRepository(self.user)
        unit_of_work = FakeIdentityUnitOfWork(repository)
        secrets = iter(("session-secret", "csrf-secret"))
        service = IdentitySessions(
            unit_of_work=lambda: unit_of_work,
            passwords=FakePasswords(),
            now=lambda: self.now,
            new_id=uuid4,
            new_secret=lambda: next(secrets),
            session_ttl=timedelta(hours=8),
        )

        result = service.create(" Creator@Example.COM ", "safe password")

        self.assertEqual(result.session_token, "session-secret")
        self.assertEqual(result.csrf_token, "csrf-secret")
        self.assertEqual(
            repository.added_session.token_hash,
            hashlib.sha256(b"session-secret").hexdigest(),
        )
        self.assertEqual(
            repository.added_session.csrf_token_hash,
            hashlib.sha256(b"csrf-secret").hexdigest(),
        )
        self.assertEqual(unit_of_work.commits, 1)

    def test_wrong_password_does_not_create_session(self) -> None:
        from thief.identity.sessions import InvalidCredentials

        repository = FakeSessionsRepository(self.user)
        service = self._service(repository)

        with self.assertRaises(InvalidCredentials):
            service.create(self.user.email, "wrong password")

        self.assertIsNone(repository.added_session)

    def test_active_session_resolves_current_identity(self) -> None:
        from thief.identity.model import Session

        session = Session(
            id=uuid4(),
            user_id=self.user.id,
            token_hash=hashlib.sha256(b"session-secret").hexdigest(),
            csrf_token_hash=hashlib.sha256(b"csrf-secret").hexdigest(),
            created_at=self.now - timedelta(minutes=1),
            expires_at=self.now + timedelta(hours=1),
        )
        service = self._service(FakeSessionsRepository(self.user, session))

        identity = service.resolve("session-secret")

        self.assertEqual((identity.user_id, identity.email), (self.user.id, self.user.email))

    def test_wrong_csrf_does_not_revoke_session(self) -> None:
        from thief.identity.model import Session
        from thief.identity.sessions import InvalidCsrf

        session = Session(
            id=uuid4(),
            user_id=self.user.id,
            token_hash=hashlib.sha256(b"session-secret").hexdigest(),
            csrf_token_hash=hashlib.sha256(b"csrf-secret").hexdigest(),
            created_at=self.now - timedelta(minutes=1),
            expires_at=self.now + timedelta(hours=1),
        )
        repository = FakeSessionsRepository(self.user, session)
        service = self._service(repository)

        with self.assertRaises(InvalidCsrf):
            service.revoke("session-secret", "wrong-csrf")

        self.assertIsNone(repository.revoked_id)

    def _service(self, repository: FakeSessionsRepository) -> object:
        from thief.identity.sessions import IdentitySessions

        return IdentitySessions(
            unit_of_work=lambda: FakeIdentityUnitOfWork(repository),
            passwords=FakePasswords(),
            now=lambda: self.now,
            new_id=uuid4,
            new_secret=lambda: "unused-secret",
            session_ttl=timedelta(hours=8),
        )


if __name__ == "__main__":
    unittest.main()
