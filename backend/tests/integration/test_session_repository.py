from __future__ import annotations

import os
import sys
import unittest
from datetime import UTC, datetime, timedelta
from pathlib import Path
from uuid import uuid4

from sqlalchemy import create_engine, text
from sqlalchemy.engine import Engine
from sqlalchemy.orm import sessionmaker


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "src"))


class SessionRepositoryIntegrationTests(unittest.TestCase):
    engine: Engine

    @classmethod
    def setUpClass(cls) -> None:
        cls.engine = create_engine(os.environ["THIEF_DATABASE_URL"])

    @classmethod
    def tearDownClass(cls) -> None:
        cls.engine.dispose()

    def test_session_create_resolve_and_revoke_round_trip(self) -> None:
        from thief.identity.passwords import Argon2PasswordHasher
        from thief.identity.sessions import IdentitySessions, InvalidSession
        from thief.identity.unit_of_work import SqlAlchemyIdentityUnitOfWork

        now = datetime.now(UTC)
        user_id = uuid4()
        email = f"session-{uuid4()}@example.com"
        passwords = Argon2PasswordHasher()
        self.addCleanup(self._delete_user, user_id)
        with self.engine.begin() as connection:
            connection.execute(
                text(
                    'INSERT INTO identity.users '
                    '(id, email, password_hash, role, created_at) '
                    'VALUES (:id, :email, :password_hash, :role, :created_at)'
                ),
                {
                    "id": user_id,
                    "email": email,
                    "password_hash": passwords.hash("safe password"),
                    "role": "creator",
                    "created_at": now,
                },
            )

        secrets = iter((f"session-{uuid4()}", f"csrf-{uuid4()}"))
        factory = sessionmaker(self.engine, expire_on_commit=False)
        sessions = IdentitySessions(
            unit_of_work=lambda: SqlAlchemyIdentityUnitOfWork(factory),
            passwords=passwords,
            now=lambda: now + timedelta(minutes=1),
            new_id=uuid4,
            new_secret=lambda: next(secrets),
            session_ttl=timedelta(hours=8),
        )

        created = sessions.create(email, "safe password")
        resolved = sessions.resolve(created.session_token)
        sessions.revoke(created.session_token, created.csrf_token)

        self.assertEqual(resolved.user_id, user_id)
        with self.assertRaises(InvalidSession):
            sessions.resolve(created.session_token)
        with self.engine.connect() as connection:
            stored = connection.execute(
                text(
                    'SELECT token_hash, csrf_token_hash, revoked_at '
                    'FROM identity.sessions WHERE user_id = :user_id'
                ),
                {"user_id": user_id},
            ).one()
        self.assertNotEqual(stored.token_hash, created.session_token)
        self.assertNotEqual(stored.csrf_token_hash, created.csrf_token)
        self.assertIsNotNone(stored.revoked_at)

    def _delete_user(self, user_id: object) -> None:
        with self.engine.begin() as connection:
            connection.execute(
                text('DELETE FROM identity.users WHERE id = :id'),
                {"id": user_id},
            )


if __name__ == "__main__":
    unittest.main()
