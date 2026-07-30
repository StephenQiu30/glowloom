from typing import Annotated, Literal
from uuid import UUID

from fastapi import APIRouter, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.production import service
from app.modules.production.contracts import TaskResponse, TaskStatus
from app.modules.production.schemas import PaginatedTasks

router = APIRouter(prefix="/api/v1/tasks", tags=["tasks"])


@router.get("", response_model=ApiResponse[PaginatedTasks])
async def list_tasks(
    workspace_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    task_type: Literal["script_extraction"] | None = None,
    status: TaskStatus | None = None,
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedTasks]:
    return ApiResponse(
        data=await service.list_tasks(
            session,
            claims,
            workspace_id,
            task_type=task_type,
            status=status,
            limit=limit or 20,
            offset=offset,
        )
    )


@router.get("/{task_id}", response_model=ApiResponse[TaskResponse])
async def get_task(
    task_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[TaskResponse]:
    return ApiResponse(data=await service.get_task(session, claims, task_id))
