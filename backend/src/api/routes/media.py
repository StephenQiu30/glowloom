from __future__ import annotations

from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Header, Request, status

from api.dependencies import database_from_request
from api.headers import validate_idempotency_key
from api.responses import RESOURCE_API_ERRORS
from schemas.common import TaskAccepted
from schemas.media_api import GenerateMediaRequest
from schemas.task_accepted import task_accepted
from services.media_generation import GenerateMediaCommand, GenerateMediaHandler

router = APIRouter(prefix="/v1")


@router.post(
    "/episodes/{episode_id}/media-generations",
    operation_id="generateMedia",
    response_model=TaskAccepted,
    status_code=status.HTTP_202_ACCEPTED,
    responses=RESOURCE_API_ERRORS,
)
async def generate_media(
    episode_id: UUID,
    body: GenerateMediaRequest,
    request: Request,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> TaskAccepted:
    value = await GenerateMediaHandler(
        database_from_request(request), release_version="0.1.0"
    ).execute(
        GenerateMediaCommand(
            episode_id=episode_id,
            usage_type=body.usage_type,
            usage_id=body.usage_id,
            input_version_id=body.input_version_id,
            idempotency_key=validate_idempotency_key(idempotency_key),
            model_profile_id=body.model_profile_id,
        )
    )
    return task_accepted(value)
