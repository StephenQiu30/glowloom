from __future__ import annotations

import sys
import unittest
from dataclasses import replace
from datetime import UTC, datetime, timedelta
from pathlib import Path
from uuid import uuid4


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "packages/core/src"))

from thief_core.identity import (  # noqa: E402
    Invitation,
    Principal,
    Role,
    Session,
    normalize_email,
)


class IdentityDomainTests(unittest.TestCase):
    def setUp(self) -> None:
        self.now = datetime(2026, 7, 22, 8, 0, tzinfo=UTC)

    def test_roles_are_the_fixed_mvp_roles(self) -> None:
        self.assertEqual({role.value for role in Role}, {"creator", "admin"})

    def test_email_is_stored_in_one_normalized_form(self) -> None:
        self.assertEqual(
            normalize_email("  Creator@Example.COM  "),
            "creator@example.com",
        )
        with self.assertRaisesRegex(ValueError, "email is required"):
            normalize_email("  ")

    def test_invitation_is_usable_only_before_expiry_and_resolution(self) -> None:
        invitation = Invitation(
            id=uuid4(),
            email="creator@example.com",
            role=Role.CREATOR,
            token_hash="a" * 64,
            invited_by=uuid4(),
            created_at=self.now,
            expires_at=self.now + timedelta(days=1),
        )

        self.assertTrue(invitation.is_usable_at(self.now))
        self.assertFalse(invitation.is_usable_at(invitation.expires_at))
        self.assertFalse(
            replace(invitation, accepted_at=self.now).is_usable_at(self.now)
        )
        self.assertFalse(
            replace(invitation, revoked_at=self.now).is_usable_at(self.now)
        )

    def test_session_is_active_only_before_expiry_and_revocation(self) -> None:
        session = Session(
            id=uuid4(),
            user_id=uuid4(),
            token_hash="b" * 64,
            csrf_token_hash="c" * 64,
            created_at=self.now,
            expires_at=self.now + timedelta(hours=8),
        )

        self.assertTrue(session.is_active_at(self.now))
        self.assertFalse(session.is_active_at(session.expires_at))
        self.assertFalse(replace(session, revoked_at=self.now).is_active_at(self.now))

    def test_admin_role_does_not_bypass_owner_isolation(self) -> None:
        user_id = uuid4()
        another_user_id = uuid4()
        principal = Principal(user_id=user_id, role=Role.ADMIN)

        self.assertTrue(principal.has_role(Role.ADMIN))
        self.assertTrue(principal.owns(user_id))
        self.assertFalse(principal.owns(another_user_id))


if __name__ == "__main__":
    unittest.main()
