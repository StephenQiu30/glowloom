"""Probe installed candidate capabilities without invoking a model."""

from __future__ import annotations

import os
import shutil
from collections.abc import Callable
from typing import Literal

from fastapi import APIRouter
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from app.modules.storygraph.bundle import BundleInvalid, StoryGraphBundle
from app.modules.storygraph.scene_analysis_bundle import SceneAnalysisBundle
from app.modules.text_storyboard.harness import RELEASE_HASH, TextSkill

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
    except (BundleInvalid, OSError, ValueError):
        return Capability(
            key=key, status="blocked", release_hash=expected, reason="skill_release_unavailable"
        )
    return Capability(key=key, status="ready", release_hash=expected)


@router.get("/readyz")
def readiness() -> JSONResponse:
    storygraph = StoryGraphBundle()
    scene_analysis = SceneAnalysisBundle()
    capabilities = [
        release_capability(
            "storygraph", storygraph.manifest.skill_bundle_hash, storygraph.verify_installed_bundle
        ),
        release_capability(
            "scene_analysis",
            scene_analysis.manifest.skill_bundle_hash,
            scene_analysis.verify_installed_bundle,
        ),
        release_capability("text_storyboard", RELEASE_HASH, TextSkill().release_hash),
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
    ready = all(item.status == "ready" for item in capabilities)
    return JSONResponse(
        status_code=200 if ready else 503,
        content={
            "status": "ready" if ready else "blocked",
            "scope": "installed_candidate_capabilities",
            "capabilities": [item.model_dump(exclude_none=True) for item in capabilities],
        },
    )
