from __future__ import annotations

import os
import time

from fastapi import APIRouter, Header, HTTPException

from app.modules.text_storyboard.harness import (
    ContextInsufficient,
    SkillReleaseInvalid,
    TextHarness,
    TextResult,
    TextTask,
)
from app.reasoning.codex import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexExecutionError,
)
from app.text_contract.authorization import verify_task

router = APIRouter()


@router.post("/internal/text-storyboard/invocations", response_model=TextResult)
async def invoke_text(
    task: TextTask,
    authorization: str = Header(alias="X-Lanverse-Text-Authorization"),
) -> TextResult:
    try:
        verify_task(task, authorization, os.getenv("AGENT_EXECUTION_SECRET", ""), int(time.time()))
    except ValueError as error:
        raise HTTPException(401, "invalid text task authorization") from error
    try:
        return await TextHarness().execute(task)
    except SkillReleaseInvalid as error:
        raise HTTPException(409, "skill_release_unavailable") from error
    except ContextInsufficient as error:
        raise HTTPException(422, "context_insufficient") from error
    except CodexDeadlineExceeded as error:
        raise HTTPException(504, "execution_deadline_exceeded") from error
    except CodexBudgetExceeded as error:
        raise HTTPException(422, "execution_output_budget_exceeded") from error
    except CodexExecutionError as error:
        raise HTTPException(502, "reasoning_execution_failed_or_unknown") from error
    except ValueError as error:
        raise HTTPException(422, "candidate_or_input_contract_invalid") from error
