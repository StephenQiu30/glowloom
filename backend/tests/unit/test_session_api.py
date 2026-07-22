from __future__ import annotations

import asyncio
import sys
import unittest
from pathlib import Path
from types import SimpleNamespace
from uuid import uuid4

import httpx


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "apps/api/src"))
sys.path.insert(0, str(BACKEND / "packages/core/src"))


class FakeIdentitySessions:
    def __init__(self) -> None:
        self.created_with: tuple[str, str] | None = None
        self.resolved_with: str | None = None
        self.revoked_with: tuple[str, str] | None = None

    def create(self, email: str, password: str) -> object:
        from thief_core.identity import Role

        self.created_with = (email, password)
        return SimpleNamespace(
            user_id=uuid4(),
            email="creator@example.com",
            role=Role.CREATOR,
            session_token="session-secret",
            csrf_token="csrf-secret",
        )

    def resolve(self, session_token: str) -> object:
        from thief_core.identity import Role

        self.resolved_with = session_token
        return SimpleNamespace(
            user_id=uuid4(),
            email="creator@example.com",
            role=Role.CREATOR,
        )

    def revoke(self, session_token: str, csrf_token: str) -> None:
        self.revoked_with = (session_token, csrf_token)


class SessionApiTests(unittest.TestCase):
    def test_session_endpoints_set_restore_and_clear_secure_cookies(self) -> None:
        asyncio.run(self._assert_session_flow())

    async def _assert_session_flow(self) -> None:
        from thief_api.main import create_app

        sessions = FakeIdentitySessions()
        transport = httpx.ASGITransport(app=create_app(sessions=sessions))
        async with httpx.AsyncClient(
            transport=transport,
            base_url="https://test",
        ) as client:
            created = await client.post(
                "/v1/session",
                json={"email": "creator@example.com", "password": "safe password"},
            )
            self.assertEqual(created.status_code, 200)
            cookies = created.headers.get_list("set-cookie")
            session_cookie = next(item for item in cookies if "thief_session=" in item)
            csrf_cookie = next(item for item in cookies if "thief_csrf=" in item)
            for value in (session_cookie, csrf_cookie):
                self.assertIn("Secure", value)
                self.assertIn("SameSite=lax", value)
            self.assertIn("HttpOnly", session_cookie)
            self.assertNotIn("HttpOnly", csrf_cookie)
            self.assertNotIn("password", created.text)

            restored = await client.get("/v1/session")
            self.assertEqual(restored.status_code, 200)
            self.assertEqual(sessions.resolved_with, "session-secret")

            ended = await client.delete(
                "/v1/session",
                headers={"X-CSRF-Token": "csrf-secret"},
            )
            self.assertEqual(ended.status_code, 204)
            self.assertEqual(
                sessions.revoked_with,
                ("session-secret", "csrf-secret"),
            )
            cleared = ended.headers.get_list("set-cookie")
            self.assertEqual(len(cleared), 2)
            self.assertTrue(all("Max-Age=0" in item for item in cleared))

    def test_invalid_credentials_use_the_common_error_contract(self) -> None:
        asyncio.run(self._assert_invalid_credentials())

    async def _assert_invalid_credentials(self) -> None:
        from thief_api.main import create_app
        from thief_core.identity import InvalidCredentials

        class RejectingSessions(FakeIdentitySessions):
            def create(self, email: str, password: str) -> object:
                raise InvalidCredentials

        transport = httpx.ASGITransport(app=create_app(sessions=RejectingSessions()))
        async with httpx.AsyncClient(
            transport=transport,
            base_url="https://test",
        ) as client:
            response = await client.post(
                "/v1/session",
                json={"email": "unknown@example.com", "password": "wrong"},
            )

        self.assertEqual(response.status_code, 401)
        self.assertEqual(response.json()["code"], "invalid_credentials")
        self.assertEqual(response.json()["details"], {})
        self.assertTrue(response.json()["trace_id"])


if __name__ == "__main__":
    unittest.main()
