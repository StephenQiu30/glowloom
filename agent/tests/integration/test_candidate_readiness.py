from __future__ import annotations

import sys
from typing import Any

import httpx
import pytest

from app.modules.text_storyboard.harness import RELEASE_HASH, TextSkill
from tests.app_factory import test_app as app


@pytest.mark.parametrize("blocked", [None, "release", "executable", "authorization"])
async def test_readiness_checks_capabilities_without_inference(
    monkeypatch: pytest.MonkeyPatch, blocked: str | None
) -> None:
    # This probe verifies executable presence, never runs it or claims model access.
    monkeypatch.setenv("CODEX_BIN", sys.executable)
    monkeypatch.setenv("AGENT_EXECUTION_SECRET", "synthetic-execution-secret-at-least-32-bytes")
    if blocked == "release":

        def drifted_release(_: TextSkill) -> str:
            return "0" * 64

        monkeypatch.setattr(TextSkill, "release_hash", drifted_release)
    elif blocked == "executable":
        monkeypatch.setenv("CODEX_BIN", "/nonexistent/synthetic-codex-private-path")
    elif blocked == "authorization":
        monkeypatch.delenv("AGENT_EXECUTION_SECRET")

    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://agent"
    ) as client:
        response = await client.get("/readyz")
    assert response.status_code == (503 if blocked else 200)
    data: dict[str, Any] = response.json()
    assert data["status"] == ("blocked" if blocked else "ready")
    capabilities = {item["key"]: item for item in data["capabilities"]}
    assert capabilities["text_storyboard"]["release_hash"] == RELEASE_HASH
    if blocked:
        key = {
            "release": "text_storyboard",
            "executable": "codex_executable",
            "authorization": "invocation_authorization",
        }[blocked]
        assert capabilities[key]["status"] == "blocked"
        assert capabilities[key]["reason"]
    assert "synthetic-execution-secret" not in response.text
    assert "synthetic-codex-private-path" not in response.text
