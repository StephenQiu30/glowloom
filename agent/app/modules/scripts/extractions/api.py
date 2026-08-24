from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.scripts.extractions import service
from app.modules.scripts.extractions.schemas import (
    CandidateDecisionRequest,
    CandidateDecisionResultResponse,
    CandidateKind,
    CandidateStatus,
    ExtractionBatchResponse,
    ExtractionCandidateResponse,
    PaginatedCandidateDecisions,
    PaginatedExtractionCandidates,
    ScriptExtractionRequest,
)

router = APIRouter(prefix="/api/v1", tags=["scripts"])


@router.post(
    "/script-versions/{version_id}/extractions",
    response_model=ApiResponse[ExtractionBatchResponse],
    status_code=status.HTTP_202_ACCEPTED,
)
async def start_extraction(
    version_id: UUID,
    payload: ScriptExtractionRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ExtractionBatchResponse]:
    return ApiResponse(
        data=await service.start_extraction(
            session,
            claims,
            version_id,
            payload,
            trace_id=request.state.request_id,
        )
    )


@router.get(
    "/extraction-batches/{batch_id}",
    response_model=ApiResponse[ExtractionBatchResponse],
)
async def get_extraction_batch(
    batch_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ExtractionBatchResponse]:
    return ApiResponse(data=await service.get_extraction_batch(session, claims, batch_id))


@router.get(
    "/extraction-batches/{batch_id}/candidates",
    response_model=ApiResponse[PaginatedExtractionCandidates],
)
async def list_extraction_candidates(
    batch_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    kind: Annotated[CandidateKind | None, Query()] = None,
    candidate_status: Annotated[
        CandidateStatus | None,
        Query(alias="status"),
    ] = None,
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedExtractionCandidates]:
    return ApiResponse(
        data=await service.list_extraction_candidates(
            session,
            claims,
            batch_id,
            kind=kind,
            status=candidate_status,
            limit=limit or 20,
            offset=offset,
        )
    )


@router.get(
    "/extraction-candidates/{candidate_id}",
    response_model=ApiResponse[ExtractionCandidateResponse],
)
async def get_extraction_candidate(
    candidate_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ExtractionCandidateResponse]:
    return ApiResponse(data=await service.get_extraction_candidate(session, claims, candidate_id))


@router.post(
    "/extraction-candidates/{candidate_id}/decisions",
    response_model=ApiResponse[CandidateDecisionResultResponse],
    status_code=status.HTTP_201_CREATED,
)
async def decide_extraction_candidate(
    candidate_id: UUID,
    payload: CandidateDecisionRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[CandidateDecisionResultResponse]:
    return ApiResponse(
        data=await service.decide_extraction_candidate(
            session,
            claims,
            candidate_id,
            payload,
        )
    )


@router.get(
    "/extraction-candidates/{candidate_id}/decisions",
    response_model=ApiResponse[PaginatedCandidateDecisions],
)
async def list_candidate_decisions(
    candidate_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedCandidateDecisions]:
    return ApiResponse(
        data=await service.list_candidate_decisions(
            session,
            claims,
            candidate_id,
            limit=limit or 20,
            offset=offset,
        )
    )
