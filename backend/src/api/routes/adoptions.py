from __future__ import annotations

from typing import Annotated

from fastapi import APIRouter, Header, Request, status

from api.dependencies import database_from_request
from api.headers import validate_idempotency_key
from api.responses import RESOURCE_API_ERRORS
from schemas.adoption_api import (
    AdoptCandidateRequest,
    AdoptionResponse,
    adoption_response,
)
from services.adoptions import AdoptCandidateCommand, AdoptCandidateHandler

router = APIRouter(prefix="/v1")


@router.post(
    "/adoptions",
    operation_id="adoptCandidate",
    response_model=AdoptionResponse,
    status_code=status.HTTP_201_CREATED,
    responses=RESOURCE_API_ERRORS,
)
async def adopt_candidate(
    body: AdoptCandidateRequest,
    request: Request,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> AdoptionResponse:
    value = await AdoptCandidateHandler(database_from_request(request)).execute(
        AdoptCandidateCommand(
            usage_type=body.usage_type,
            usage_id=body.usage_id,
            input_version_id=body.input_version_id,
            input_hash=body.input_hash,
            candidate_id=body.candidate_id,
            idempotency_key=validate_idempotency_key(idempotency_key),
        )
    )
    return adoption_response(value)
