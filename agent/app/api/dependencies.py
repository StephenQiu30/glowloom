"""FastAPI dependency providers for the single Agent application."""

from __future__ import annotations

from typing import Annotated

from fastapi import Depends, HTTPException, Request

from app.core.container import AgentRuntime
from app.creation.service import CreationService
from app.harness.service import HarnessService
from app.skills.catalog import SkillCatalog
from app.skills.runtime import SkillRuntime

MAX_AUTHORIZED_BODY_BYTES = 16384


def get_runtime(request: Request) -> AgentRuntime:
    return request.app.state.runtime


def get_skill_catalog(runtime: Annotated[AgentRuntime, Depends(get_runtime)]) -> SkillCatalog:
    return runtime.skill_runtime.catalog


def get_skill_runtime(runtime: Annotated[AgentRuntime, Depends(get_runtime)]) -> SkillRuntime:
    return runtime.skill_runtime


def get_creation_service(runtime: Annotated[AgentRuntime, Depends(get_runtime)]) -> CreationService:
    return CreationService(
        repository=runtime.repository,
        task_queue=runtime.task_queue,
        temporal=runtime.temporal,
    )


def get_harness_service(
    skill_runtime: Annotated[SkillRuntime, Depends(get_skill_runtime)],
) -> HarnessService:
    return HarnessService(skill_runtime)


async def authorized_body(
    request: Request, runtime: Annotated[AgentRuntime, Depends(get_runtime)]
) -> bytes:
    """Read and authenticate the exact request body used by Creation routes."""

    from app.creation.authorization import HEADER, InvalidAuthorization, verify_authorization

    raw = bytearray()
    async for chunk in request.stream():
        if len(raw) + len(chunk) > MAX_AUTHORIZED_BODY_BYTES:
            raise HTTPException(413, "creation_command_too_large")
        raw.extend(chunk)
    try:
        tokens = request.headers.getlist(HEADER)
        if len(tokens) != 1 or request.url.query:
            raise InvalidAuthorization("missing or ambiguous authorization")
        path = request.scope["raw_path"].decode("ascii")
        verify_authorization(tokens[0], runtime.secret, request.method, path, bytes(raw))
    except (InvalidAuthorization, UnicodeError) as error:
        raise HTTPException(401, "creation_authorization_invalid") from error
    return bytes(raw)
