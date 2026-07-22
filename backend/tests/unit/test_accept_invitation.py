from __future__ import annotations

import hashlib
import sys
import unittest
from datetime import UTC, datetime, timedelta
from pathlib import Path
from uuid import UUID, uuid4


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "packages/core/src"))


class FakePasswordHasher:
    def __init__(self) -> None:
        self.values: list[str] = []

    def hash(self, value: str) -> str:
        self.values.append(value)
        return f"hashed:{value}"

    def verify(self, encoded: str, value: str) -> bool:
        return encoded == f"hashed:{value}"


class FakeIdentityRepository:
    def __init__(self, invitation: object, *, email_exists: bool = False) -> None:
        self.invitation = invitation
        self.email_exists_result = email_exists
        self.added_user: object | None = None
        self.accepted_id: UUID | None = None
        self.seen_token_hash: str | None = None

    def find_invitation(self, token_hash: str) -> object | None:
        self.seen_token_hash = token_hash
        expected = getattr(self.invitation, "token_hash")
        return self.invitation if token_hash == expected else None

    def email_exists(self, email: str) -> bool:
        return self.email_exists_result

    def add_user(self, user: object) -> None:
        self.added_user = user

    def accept_invitation(self, invitation_id: UUID, accepted_at: datetime) -> bool:
        self.accepted_id = invitation_id
        return True


class FakeIdentityUnitOfWork:
    def __init__(self, repository: FakeIdentityRepository) -> None:
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


class AcceptInvitationTests(unittest.TestCase):
    def setUp(self) -> None:
        from thief_core.identity import Invitation, Role

        self.now = datetime(2026, 7, 22, 9, 0, tzinfo=UTC)
        self.invitation = Invitation(
            id=uuid4(),
            email="creator@example.com",
            role=Role.CREATOR,
            token_hash=hashlib.sha256(b"invite-token").hexdigest(),
            invited_by=uuid4(),
            created_at=self.now - timedelta(hours=1),
            expires_at=self.now + timedelta(hours=1),
        )

    def test_valid_invitation_creates_one_user_and_is_resolved(self) -> None:
        from thief_core.identity import AcceptInvitation, AcceptInvitationCommand

        repository = FakeIdentityRepository(self.invitation)
        unit_of_work = FakeIdentityUnitOfWork(repository)
        passwords = FakePasswordHasher()
        user_id = uuid4()
        use_case = AcceptInvitation(
            unit_of_work=lambda: unit_of_work,
            passwords=passwords,
            now=lambda: self.now,
            new_id=lambda: user_id,
        )

        result = use_case.execute(
            AcceptInvitationCommand(token="invite-token", password="safe password")
        )

        self.assertEqual((result.user_id, result.email), (user_id, self.invitation.email))
        self.assertEqual(passwords.values, ["safe password"])
        self.assertEqual(repository.seen_token_hash, self.invitation.token_hash)
        self.assertEqual(repository.accepted_id, self.invitation.id)
        self.assertEqual(unit_of_work.commits, 1)
        self.assertEqual(repository.added_user.password_hash, "hashed:safe password")

    def test_expired_invitation_does_not_hash_or_write(self) -> None:
        from thief_core.identity import (
            AcceptInvitation,
            AcceptInvitationCommand,
            InvitationNotUsable,
        )

        repository = FakeIdentityRepository(self.invitation)
        passwords = FakePasswordHasher()
        use_case = AcceptInvitation(
            unit_of_work=lambda: FakeIdentityUnitOfWork(repository),
            passwords=passwords,
            now=lambda: self.invitation.expires_at,
            new_id=uuid4,
        )

        with self.assertRaises(InvitationNotUsable):
            use_case.execute(AcceptInvitationCommand("invite-token", "safe password"))

        self.assertEqual(passwords.values, [])
        self.assertIsNone(repository.added_user)

    def test_existing_email_does_not_create_a_second_user(self) -> None:
        from thief_core.identity import (
            AcceptInvitation,
            AcceptInvitationCommand,
            EmailAlreadyRegistered,
        )

        repository = FakeIdentityRepository(self.invitation, email_exists=True)
        use_case = AcceptInvitation(
            unit_of_work=lambda: FakeIdentityUnitOfWork(repository),
            passwords=FakePasswordHasher(),
            now=lambda: self.now,
            new_id=uuid4,
        )

        with self.assertRaises(EmailAlreadyRegistered):
            use_case.execute(AcceptInvitationCommand("invite-token", "safe password"))

        self.assertIsNone(repository.added_user)


if __name__ == "__main__":
    unittest.main()
