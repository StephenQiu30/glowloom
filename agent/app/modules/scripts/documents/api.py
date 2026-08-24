from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import (
    AccessTokenClaims,
    get_access_token_claims,
    get_request_settings,
)
from app.core.config import Settings
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.integrations.dependencies import get_media_storage
from app.modules.media import MediaStorage
from app.modules.scripts.documents import service
from app.modules.scripts.documents.schemas import (
    PaginatedScriptDocuments,
    ScriptDocumentAnalysisResponse,
    ScriptDocumentImportRequest,
    ScriptDocumentPreviewRequest,
    ScriptDocumentPreviewResponse,
)

router = APIRouter(prefix="/api/v1", tags=["script-documents"])


@router.post(
    "/projects/{project_id}/script-import-previews",
    response_model=ApiResponse[ScriptDocumentPreviewResponse],
)
async def preview_document(
    project_id: UUID,
    payload: ScriptDocumentPreviewRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    storage: Annotated[MediaStorage, Depends(get_media_storage)],
    settings: Annotated[Settings, Depends(get_request_settings)],
) -> ApiResponse[ScriptDocumentPreviewResponse]:
    return ApiResponse(
        data=await service.preview_document(
            session,
            claims,
            project_id,
            payload,
            storage,
            settings,
        )
    )


@router.post(
    "/projects/{project_id}/script-imports",
    response_model=ApiResponse[ScriptDocumentAnalysisResponse],
    status_code=status.HTTP_201_CREATED,
)
async def import_document(
    project_id: UUID,
    payload: ScriptDocumentImportRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    storage: Annotated[MediaStorage, Depends(get_media_storage)],
    settings: Annotated[Settings, Depends(get_request_settings)],
) -> ApiResponse[ScriptDocumentAnalysisResponse]:
    return ApiResponse(
        data=await service.import_document(
            session,
            claims,
            project_id,
            payload,
            storage,
            settings,
            trace_id=str(request.state.request_id),
        )
    )


@router.get(
    "/document-revisions/{revision_id}",
    response_model=ApiResponse[ScriptDocumentAnalysisResponse],
)
async def get_revision(
    revision_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ScriptDocumentAnalysisResponse]:
    return ApiResponse(data=await service.get_revision(session, claims, revision_id))


@router.get(
    "/projects/{project_id}/script-documents",
    response_model=ApiResponse[PaginatedScriptDocuments],
)
async def list_documents(
    project_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedScriptDocuments]:
    return ApiResponse(
        data=await service.list_documents(
            session,
            claims,
            project_id,
            limit=limit or 20,
            offset=offset,
        )
    )
