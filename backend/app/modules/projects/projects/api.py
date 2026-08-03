from typing import Annotated, Literal
from uuid import UUID

from fastapi import APIRouter, Depends, Query, Request, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims, get_access_token_claims
from app.core.database import get_async_session
from app.core.schemas import ApiResponse
from app.modules.assets import summarize_project_asset_references
from app.modules.projects.contracts import (
    DeletePreflightResponse,
    DeleteResponse,
    EpisodeStoryboardReferenceSummary,
    ProjectAssetReferenceSummary,
)
from app.modules.projects.projects import service
from app.modules.projects.projects.schemas import (
    BudgetLimitRequest,
    PaginatedProjects,
    ProjectCreateRequest,
    ProjectResponse,
    ProjectStateRequest,
    ProjectUpdateRequest,
)
from app.modules.scripts import count_episode_script_versions
from app.modules.storyboards import summarize_episode_storyboard_references

router = APIRouter(prefix="/api/v1", tags=["projects"])


@router.get("/projects", response_model=ApiResponse[PaginatedProjects])
async def list_projects(
    workspace_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
    include_archived: bool = False,
    search: Annotated[str | None, Query(max_length=120)] = None,
    sort: Literal["name", "created_at", "updated_at"] | None = None,
    order: Literal["asc", "desc"] | None = None,
    limit: Annotated[int | None, Query(ge=1, le=100)] = None,
    offset: Annotated[int, Query(ge=0)] = 0,
) -> ApiResponse[PaginatedProjects]:
    return ApiResponse(
        data=await service.list_projects(
            session,
            claims,
            workspace_id,
            include_archived=include_archived,
            search=search,
            sort=sort or "updated_at",
            order=order or "desc",
            limit=limit or 20,
            offset=offset,
        )
    )


@router.post(
    "/projects",
    response_model=ApiResponse[ProjectResponse],
    status_code=status.HTTP_201_CREATED,
)
async def create_project(
    payload: ProjectCreateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProjectResponse]:
    return ApiResponse(
        data=await service.create_project(
            session, claims, payload, trace_id=str(request.state.request_id)
        )
    )


@router.get("/projects/{project_id}", response_model=ApiResponse[ProjectResponse])
async def get_project(
    project_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProjectResponse]:
    return ApiResponse(data=await service.get_project(session, claims, project_id))


@router.patch("/projects/{project_id}", response_model=ApiResponse[ProjectResponse])
async def update_project(
    project_id: UUID,
    payload: ProjectUpdateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProjectResponse]:
    return ApiResponse(
        data=await service.update_project(
            session,
            claims,
            project_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/projects/{project_id}/budget-limit",
    response_model=ApiResponse[ProjectResponse],
)
async def update_budget_limit(
    project_id: UUID,
    payload: BudgetLimitRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProjectResponse]:
    return ApiResponse(
        data=await service.update_budget(
            session,
            claims,
            project_id,
            payload,
            trace_id=str(request.state.request_id),
        )
    )


@router.post("/projects/{project_id}/archive", response_model=ApiResponse[ProjectResponse])
async def archive_project(
    project_id: UUID,
    payload: ProjectStateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProjectResponse]:
    return ApiResponse(
        data=await service.set_archived(
            session,
            claims,
            project_id,
            payload,
            archived=True,
            trace_id=str(request.state.request_id),
        )
    )


@router.post("/projects/{project_id}/restore", response_model=ApiResponse[ProjectResponse])
async def restore_project(
    project_id: UUID,
    payload: ProjectStateRequest,
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[ProjectResponse]:
    return ApiResponse(
        data=await service.set_archived(
            session,
            claims,
            project_id,
            payload,
            archived=False,
            trace_id=str(request.state.request_id),
        )
    )


@router.post(
    "/projects/{project_id}/delete-preflight",
    response_model=ApiResponse[DeletePreflightResponse],
)
async def delete_preflight(
    project_id: UUID,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DeletePreflightResponse]:
    async def read_script_version_counts(
        *, workspace_id: UUID, episode_ids: list[UUID]
    ) -> dict[UUID, int]:
        return await count_episode_script_versions(session, workspace_id, episode_ids)

    async def read_storyboard_references(
        *, workspace_id: UUID, episode_ids: list[UUID]
    ) -> dict[UUID, EpisodeStoryboardReferenceSummary]:
        summaries = await summarize_episode_storyboard_references(
            session,
            workspace_id,
            episode_ids,
        )
        return {
            episode_id: EpisodeStoryboardReferenceSummary(
                shot_count=summary.shot_count,
                spec_version_count=summary.spec_version_count,
            )
            for episode_id, summary in summaries.items()
        }

    async def read_asset_references(
        *, workspace_id: UUID, project_ids: list[UUID]
    ) -> dict[UUID, ProjectAssetReferenceSummary]:
        summaries = await summarize_project_asset_references(
            session,
            workspace_id,
            project_ids,
        )
        return {
            project_id: ProjectAssetReferenceSummary(
                asset_count=summary.asset_count,
                version_count=summary.version_count,
            )
            for project_id, summary in summaries.items()
        }

    return ApiResponse(
        data=await service.delete_preflight(
            session,
            claims,
            project_id,
            read_script_version_counts,
            read_storyboard_references,
            read_asset_references,
        )
    )


@router.delete("/projects/{project_id}", response_model=ApiResponse[DeleteResponse])
async def delete_project(
    project_id: UUID,
    expected_revision: Annotated[int, Query(ge=1)],
    request: Request,
    claims: Annotated[AccessTokenClaims, Depends(get_access_token_claims)],
    session: Annotated[AsyncSession, Depends(get_async_session)],
) -> ApiResponse[DeleteResponse]:
    async def read_asset_references(
        *, workspace_id: UUID, project_ids: list[UUID]
    ) -> dict[UUID, ProjectAssetReferenceSummary]:
        summaries = await summarize_project_asset_references(
            session,
            workspace_id,
            project_ids,
        )
        return {
            project_id: ProjectAssetReferenceSummary(
                asset_count=summary.asset_count,
                version_count=summary.version_count,
            )
            for project_id, summary in summaries.items()
        }

    await service.delete_project(
        session,
        claims,
        project_id,
        expected_revision,
        read_asset_references,
        trace_id=str(request.state.request_id),
    )
    return ApiResponse(data=DeleteResponse())
