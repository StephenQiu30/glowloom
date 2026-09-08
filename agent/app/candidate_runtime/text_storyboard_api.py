from __future__ import annotations

import hashlib
import hmac
import os
import time

from fastapi import APIRouter, Header, HTTPException

from app.candidate_runtime.canonical import canonical_hash
from app.modules.storygraph.harness import (
    CodexBudgetExceeded,
    CodexDeadlineExceeded,
    CodexExecutionError,
)
from app.modules.text_storyboard.harness import (
    ContextInsufficient,
    SkillReleaseInvalid,
    TextHarness,
    TextResult,
    TextTask,
)

router = APIRouter()
_AUDIENCE = "lanverse.text-storyboard.invocation"


def sign_task(task: TextTask, secret: str, expires_at: int) -> str:
    if len(secret.encode()) < 32:
        raise ValueError("text task signing secret must contain at least 32 bytes")
    message = f"{_AUDIENCE}\n{expires_at}\n{canonical_hash(task.model_dump(mode='json'))}"
    signature = hmac.new(secret.encode(), message.encode(), hashlib.sha256).hexdigest()
    return f"{expires_at}.{signature}"


def verify_task(task: TextTask, token: str, secret: str, now: int) -> None:
    try:
        expires, signature = token.split(".", 1)
        expiry = int(expires)
        if str(expiry) != expires or not now < expiry <= now + 60:
            raise ValueError("invalid authorization window")
        expected = sign_task(task, secret, expiry)
        if len(signature) != 64 or not hmac.compare_digest(expected, token):
            raise ValueError("invalid authorization")
    except (ValueError, UnicodeError) as error:
        raise ValueError("invalid text task authorization") from error


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
