from __future__ import annotations

import hashlib
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


class IdentityRepositoryIntegrationTests(unittest.TestCase):
    engine: Engine

    @classmethod
    def setUpClass(cls) -> None:
        cls.engine = create_engine(os.environ["THIEF_DATABASE_URL"])

    @classmethod
    def tearDownClass(cls) -> None:
        cls.engine.dispose()

    def test_accept_invitation_persists_one_user_without_plaintext(self) -> None:
        from thief.identity.invitations import (
            AcceptInvitation,
            AcceptInvitationCommand,
        )
        from thief.identity.passwords import Argon2PasswordHasher
        from thief.identity.unit_of_work import SqlAlchemyIdentityUnitOfWork

        now = datetime.now(UTC)
        inviter_id = uuid4()
        invitation_id = uuid4()
        token = f"invite-{uuid4()}"
        email = f"creator-{uuid4()}@example.com"
        token_hash = hashlib.sha256(token.encode()).hexdigest()
        self.addCleanup(self._delete_fixture, inviter_id, email)
        with self.engine.begin() as connection:
            connection.execute(
                text(
                    'INSERT INTO identity.users '
                    '(id, email, password_hash, role, created_at) '
                    'VALUES (:id, :email, :password_hash, :role, :created_at)'
                ),
                {
                    "id": inviter_id,
                    "email": f"admin-{uuid4()}@example.com",
                    "password_hash": "$argon2id$bootstrap",
                    "role": "admin",
                    "created_at": now,
                },
            )
            connection.execute(
                text(
                    'INSERT INTO identity.invitations '
                    '(id, email, token_hash, role, invited_by, created_at, expires_at) '
                    'VALUES (:id, :email, :token_hash, :role, :invited_by, '
                    ':created_at, :expires_at)'
                ),
                {
                    "id": invitation_id,
                    "email": email,
                    "token_hash": token_hash,
                    "role": "creator",
                    "invited_by": inviter_id,
                    "created_at": now,
                    "expires_at": now + timedelta(hours=1),
                },
            )

        factory = sessionmaker(self.engine, expire_on_commit=False)
        use_case = AcceptInvitation(
            unit_of_work=lambda: SqlAlchemyIdentityUnitOfWork(factory),
            passwords=Argon2PasswordHasher(),
            now=lambda: now + timedelta(minutes=1),
            new_id=uuid4,
        )
        result = use_case.execute(AcceptInvitationCommand(token, "safe password"))

        with self.engine.connect() as connection:
            user = connection.execute(
                text(
                    'SELECT id, password_hash FROM identity.users '
                    'WHERE email = :email'
                ),
                {"email": email},
            ).one()
            accepted_at = connection.execute(
                text(
                    'SELECT accepted_at FROM identity.invitations WHERE id = :id'
                ),
                {"id": invitation_id},
            ).scalar_one()

        self.assertEqual(user.id, result.user_id)
        self.assertTrue(user.password_hash.startswith("$argon2id$"))
        self.assertNotIn("safe password", user.password_hash)
        self.assertNotIn(token, user.password_hash)
        self.assertIsNotNone(accepted_at)

    def _delete_fixture(self, inviter_id: object, email: str) -> None:
        with self.engine.begin() as connection:
            connection.execute(
                text('DELETE FROM identity.invitations WHERE invited_by = :id'),
                {"id": inviter_id},
            )
            connection.execute(
                text('DELETE FROM identity.users WHERE email = :email OR id = :id'),
                {"email": email, "id": inviter_id},
            )


if __name__ == "__main__":
    unittest.main()
