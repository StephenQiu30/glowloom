from __future__ import annotations

from typing import Annotated, Any
from uuid import UUID

from fastapi import APIRouter, Header, Query, Request, Response

from api.dependencies import database_from_request
from api.headers import parse_if_match, strong_etag
from schemas.common import Problem
from schemas.storyboard_api import (
    CreativeAssetListResponse,
    CreativeAssetVersionResponse,
    SaveCreativeAssetRequest,
    asset_response,
)
from services.storyboards import (
    GetCreativeAssetVersionHandler,
    ListCreativeAssetsHandler,
    SaveCreativeAssetCommand,
    SaveCreativeAssetHandler,
)

router = APIRouter(prefix="/v1")
ERROR_RESPONSES: dict[int | str, dict[str, Any]] = {
    404: {"model": Problem},
    409: {"model": Problem},
    412: {"model": Problem},
    422: {"model": Problem},
}


@router.get(
    "/episodes/{episode_id}/creative-assets",
    operation_id="listCreativeAssets",
    response_model=CreativeAssetListResponse,
    responses=ERROR_RESPONSES,
)
async def list_creative_assets(
    episode_id: UUID,
    request: Request,
    include_versions: Annotated[bool, Query()],
) -> CreativeAssetListResponse:
    del include_versions
    values = await ListCreativeAssetsHandler(database_from_request(request)).execute(episode_id)
    return CreativeAssetListResponse(items=tuple(asset_response(item) for item in values))


@router.get(
    "/creative-asset-versions/{version_id}",
    operation_id="getCreativeAssetVersion",
    response_model=CreativeAssetVersionResponse,
    responses=ERROR_RESPONSES,
)
async def get_creative_asset_version(
    version_id: UUID, request: Request, response: Response
) -> CreativeAssetVersionResponse:
    value = await GetCreativeAssetVersionHandler(database_from_request(request)).execute(
        version_id
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return asset_response(value)


@router.put(
    "/creative-asset-versions/{version_id}",
    operation_id="saveCreativeAsset",
    response_model=CreativeAssetVersionResponse,
    responses=ERROR_RESPONSES,
)
async def save_creative_asset(
    version_id: UUID,
    body: SaveCreativeAssetRequest,
    request: Request,
    response: Response,
    if_match: Annotated[str, Header(alias="If-Match")],
) -> CreativeAssetVersionResponse:
    value = await SaveCreativeAssetHandler(database_from_request(request)).execute(
        SaveCreativeAssetCommand(version_id, parse_if_match(if_match), body.content)
    )
    response.headers["ETag"] = strong_etag(value.resource_version)
    return asset_response(value)
