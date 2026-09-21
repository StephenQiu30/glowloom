"""Text Storyboard HTTP authentication and response mapping."""

from __future__ import annotations

import os
import time
from typing import Annotated, Any

from fastapi import APIRouter, Depends, Header, HTTPException

from app.api.dependencies import get_harness_service
from app.harness.service import HarnessService
from app.text_contract.authorization import verify_task
from app.text_contract.failure import HarnessFailed
from app.text_contract.task import TextResult, TextTask

router = APIRouter(tags=["text-storyboard"])
Service = Annotated[HarnessService, Depends(get_harness_service)]


@router.post("/internal/text-storyboard/invocations", response_model=TextResult)
async def invoke_text(
    task: TextTask,
    service: Service,
    authorization: str = Header(alias="X-Lanverse-Text-Authorization"),
) -> dict[str, Any]:
    try:
        verify_task(task, authorization, os.getenv("AGENT_EXECUTION_SECRET", ""), int(time.time()))
    except ValueError as error:
        raise HTTPException(status_code=401, detail="invalid text task authorization") from error
    try:
        return await service.invoke(task)
    except HarnessFailed as error:
        failure = error.failure
        status = (
            502
            if failure.state == "unknown"
            else (409 if failure.code == "skill_release_unavailable" else 422)
        )
        raise HTTPException(status, failure.model_dump(mode="json")) from None
