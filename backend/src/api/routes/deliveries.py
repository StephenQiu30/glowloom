from __future__ import annotations

from typing import Annotated
from uuid import UUID

from fastapi import APIRouter, Header, Request, status

from api.dependencies import (
    clock_from_request,
    database_from_request,
    object_store_from_request,
)
from api.headers import validate_idempotency_key
from api.responses import DELIVERY_API_ERRORS
from schemas.common import TaskAccepted
from schemas.delivery_api import (
    DeliveryDetailResponse,
    DeliveryListResponse,
    DownloadAuthorizationRequest,
    DownloadAuthorizationResponse,
)
from schemas.delivery_api_mappers import (
    authorization_response,
    detail_response,
    summary_response,
)
from schemas.task_accepted import task_accepted
from services.deliveries import DeliveryDownloadService, DeliveryQueryService
from services.render_recipe import pinned_render_recipe
from services.render_submission import RenderEpisodeCommand, RenderEpisodeCoordinator

router = APIRouter(prefix="/v1")


@router.post(
    "/episodes/{episode_id}/renders",
    operation_id="renderEpisode",
    response_model=TaskAccepted,
    status_code=status.HTTP_202_ACCEPTED,
    responses=DELIVERY_API_ERRORS,
)
async def render_episode(
    episode_id: UUID,
    request: Request,
    idempotency_key: Annotated[str, Header(alias="Idempotency-Key")],
) -> TaskAccepted:
    runtime = request.app.state.runtime
    value = await RenderEpisodeCoordinator(
        database_from_request(request),
        recipe=pinned_render_recipe(runtime.settings.render_runtime_image),
        release_version="0.1.0",
    ).execute(
        RenderEpisodeCommand(
            episode_id=episode_id,
            idempotency_key=validate_idempotency_key(idempotency_key),
        )
    )
    return task_accepted(value)


@router.get(
    "/episodes/{episode_id}/deliveries",
    operation_id="listDeliveries",
    response_model=DeliveryListResponse,
    responses=DELIVERY_API_ERRORS,
)
async def list_deliveries(episode_id: UUID, request: Request) -> DeliveryListResponse:
    values = await DeliveryQueryService(database_from_request(request)).list(episode_id)
    return DeliveryListResponse(items=tuple(summary_response(item) for item in values))


@router.get(
    "/deliveries/{delivery_id}",
    operation_id="getDelivery",
    response_model=DeliveryDetailResponse,
    responses=DELIVERY_API_ERRORS,
)
async def get_delivery(delivery_id: UUID, request: Request) -> DeliveryDetailResponse:
    value = await DeliveryQueryService(database_from_request(request)).get(delivery_id)
    return detail_response(value)


@router.post(
    "/deliveries/{delivery_id}/download-authorizations",
    operation_id="authorizeDownload",
    response_model=DownloadAuthorizationResponse,
    responses=DELIVERY_API_ERRORS,
)
async def authorize_download(
    delivery_id: UUID,
    body: DownloadAuthorizationRequest,
    request: Request,
) -> DownloadAuthorizationResponse:
    values = await DeliveryDownloadService(
        database_from_request(request),
        object_store_from_request(request),
        clock_from_request(request),
    ).authorize(
        delivery_id=delivery_id,
        episode_id=body.episode_id,
        artifact_types=body.artifact_types,
    )
    return authorization_response(values)
