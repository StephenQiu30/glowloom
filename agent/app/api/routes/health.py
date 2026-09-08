"""Health and readiness endpoints for the Agent process."""

from __future__ import annotations

from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException
from fastapi.responses import JSONResponse

from app.api.dependencies import get_creation_service, get_runtime, get_skill_catalog
from app.core.container import AgentRuntime
from app.creation.service import CreationService
from app.harness.readiness import collect_capabilities
from app.skills.catalog import SkillCatalog, SkillUnavailable

router = APIRouter(tags=["health"])


@router.get("/healthz")
async def healthz(catalog: Annotated[SkillCatalog, Depends(get_skill_catalog)]) -> dict[str, str]:
    try:
        verified = catalog.verify_all()
    except SkillUnavailable as error:
        raise HTTPException(status_code=503, detail="skill_release_unavailable") from error
    return {
        "status": "ok",
        "service": "lanverse-agent",
        "skill_bundle_hash": verified["storygraph"],
        "scene_analysis_skill_bundle_hash": verified["scene_analysis"],
        "text_storyboard_skill_bundle_hash": verified["text_storyboard"],
    }


@router.get("/readyz")
async def readyz(
    runtime: Annotated[AgentRuntime, Depends(get_runtime)],
    creation: Annotated[CreationService, Depends(get_creation_service)],
    catalog: Annotated[SkillCatalog, Depends(get_skill_catalog)],
) -> JSONResponse:
    await creation.ready()
    if runtime.runtime_ready is not None and not runtime.runtime_ready():
        raise HTTPException(status_code=503, detail="agent_runtime_unavailable")
    capabilities = collect_capabilities(catalog)
    ready = all(item.status == "ready" for item in capabilities)
    return JSONResponse(
        status_code=200 if ready else 503,
        content={
            "status": "ready" if ready else "blocked",
            "scope": "agent_runtime",
            "capabilities": [item.model_dump(exclude_none=True) for item in capabilities],
        },
    )
