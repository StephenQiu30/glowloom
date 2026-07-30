from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Depends, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.identity.workspaces import service
from app.modules.identity.workspaces.schemas import (
    WorkspaceCreateRequest,
    WorkspaceResponse,
    WorkspaceStateRequest,
    WorkspaceUpdateRequest,
)

router = APIRouter(prefix="/api/v1", tags=["identity"])


@router.get("/workspaces", response_model=ApiResponse[list[WorkspaceResponse]])
async def list_workspaces(
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    include_archived: bool = False,
) -> ApiResponse[list[WorkspaceResponse]]:
    return ApiResponse(
        data=await service.list_workspaces(
            session,
            claims,
            include_archived=include_archived,
        )
    )


@router.post(
    "/workspaces",
    response_model=ApiResponse[WorkspaceResponse],
    status_code=status.HTTP_201_CREATED,
)
async def create_workspace(
    payload: WorkspaceCreateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[WorkspaceResponse]:
    return ApiResponse(data=await service.create_workspace(session, claims, payload))


@router.get("/workspaces/{workspace_id}", response_model=ApiResponse[WorkspaceResponse])
async def get_workspace(
    workspace_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[WorkspaceResponse]:
    return ApiResponse(data=await service.get_workspace(session, claims, workspace_id))


@router.patch("/workspaces/{workspace_id}", response_model=ApiResponse[WorkspaceResponse])
async def update_workspace(
    workspace_id: UUID,
    payload: WorkspaceUpdateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[WorkspaceResponse]:
    return ApiResponse(data=await service.update_workspace(session, claims, workspace_id, payload))


@router.post(
    "/workspaces/{workspace_id}/archive",
    response_model=ApiResponse[WorkspaceResponse],
)
async def archive_workspace(
    workspace_id: UUID,
    payload: WorkspaceStateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[WorkspaceResponse]:
    return ApiResponse(
        data=await service.set_workspace_archived(
            session,
            claims,
            workspace_id,
            payload,
            archived=True,
        )
    )


@router.post(
    "/workspaces/{workspace_id}/restore",
    response_model=ApiResponse[WorkspaceResponse],
)
async def restore_workspace(
    workspace_id: UUID,
    payload: WorkspaceStateRequest,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[WorkspaceResponse]:
    return ApiResponse(
        data=await service.set_workspace_archived(
            session,
            claims,
            workspace_id,
            payload,
            archived=False,
        )
    )
