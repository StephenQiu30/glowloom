"""Probe installed candidate capabilities without invoking a model."""

from __future__ import annotations

import os
import shutil
from collections.abc import Callable
from typing import Literal

from fastapi import APIRouter
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from app.skills.catalog import SkillCatalog, SkillUnavailable

router = APIRouter()


class Capability(BaseModel):
    key: str
    status: Literal["ready", "blocked"]
    release_hash: str | None = None
    reason: str | None = None


def release_capability(key: str, expected: str, verify: Callable[[], str]) -> Capability:
    try:
        if verify() != expected:
            raise ValueError("release drift")
    except (SkillUnavailable, OSError, ValueError):
        return Capability(
            key=key, status="blocked", release_hash=expected, reason="skill_release_unavailable"
        )
    return Capability(key=key, status="ready", release_hash=expected)


@router.get("/readyz")
def readiness() -> JSONResponse:
    capabilities = collect_capabilities()
    ready = all(item.status == "ready" for item in capabilities)
    return JSONResponse(
        status_code=200 if ready else 503,
        content={
            "status": "ready" if ready else "blocked",
            "scope": "installed_candidate_capabilities",
            "capabilities": [item.model_dump(exclude_none=True) for item in capabilities],
        },
    )


def candidate_runtime_ready() -> bool:
    """Return whether all local candidate execution prerequisites are installed."""

    return all(item.status == "ready" for item in collect_capabilities())


def collect_capabilities() -> list[Capability]:
    catalog = SkillCatalog()
    capabilities = [
        release_capability(
            registration.key,
            registration.expected_hash,
            lambda registration=registration: catalog.verify(registration.key),
        )
        for registration in catalog.registrations()
    ]
    executable = shutil.which(os.getenv("CODEX_BIN", "").strip() or "codex")
    capabilities.append(
        Capability(
            key="codex_executable",
            status="ready" if executable else "blocked",
            reason=None if executable else "codex_executable_unavailable",
        )
    )
    authorized = len(os.getenv("AGENT_EXECUTION_SECRET", "").encode()) >= 32
    capabilities.append(
        Capability(
            key="invocation_authorization",
            status="ready" if authorized else "blocked",
            reason=None if authorized else "execution_secret_unconfigured",
        )
    )
    return capabilities
