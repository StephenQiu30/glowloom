"""Creation HTTP contract routes."""

from __future__ import annotations

from typing import Annotated

from fastapi import APIRouter, Depends, HTTPException
from fastapi.encoders import jsonable_encoder
from fastapi.responses import JSONResponse

from app.api.dependencies import authorized_body, get_creation_service
from app.creation.service import CreationRequestError, CreationService

router = APIRouter(prefix="/internal/creation", tags=["creation"])

Body = Annotated[bytes, Depends(authorized_body)]
Service = Annotated[CreationService, Depends(get_creation_service)]


def _error(error: CreationRequestError) -> HTTPException:
    return HTTPException(status_code=error.status_code, detail=error.detail)


@router.post("/commands")
async def accept_command(raw: Body, service: Service) -> JSONResponse:
    try:
        receipt = await service.accept(raw)
    except CreationRequestError as error:
        raise _error(error) from error
    return JSONResponse(receipt.model_dump(mode="json", by_alias=True), status_code=202)


@router.get("/commands/{command_id}")
async def lookup_command(command_id: str, raw: Body, service: Service) -> JSONResponse:
    try:
        receipt = await service.lookup(command_id, raw)
    except CreationRequestError as error:
        raise _error(error) from error
    return JSONResponse(receipt.model_dump(mode="json", by_alias=True))


@router.post("/commands/{command_id}/resume")
async def resume_command(command_id: str, raw: Body, service: Service) -> JSONResponse:
    try:
        value = await service.resume(command_id, raw)
    except CreationRequestError as error:
        raise _error(error) from error
    return JSONResponse(value, status_code=202)


@router.get("/commands/{command_id}/execution")
async def execution_snapshot(command_id: str, raw: Body, service: Service) -> JSONResponse:
    try:
        value = await service.execution_snapshot(command_id, raw)
    except CreationRequestError as error:
        raise _error(error) from error
    return JSONResponse(jsonable_encoder(value))


@router.get("/commands/{command_id}/drafts/{draft_id}")
async def draft(command_id: str, draft_id: str, raw: Body, service: Service) -> JSONResponse:
    try:
        value = await service.draft(command_id, draft_id, raw)
    except CreationRequestError as error:
        raise _error(error) from error
    return JSONResponse(jsonable_encoder(value))


@router.get("/commands/{command_id}/steps/{step_id}/attempts")
async def attempts(command_id: str, step_id: str, raw: Body, service: Service) -> JSONResponse:
    try:
        value = await service.attempts(command_id, step_id, raw)
    except CreationRequestError as error:
        raise _error(error) from error
    return JSONResponse(jsonable_encoder(value))


@router.get("/commands/{command_id}/manifest")
async def manifest(command_id: str, raw: Body, service: Service) -> JSONResponse:
    try:
        value = await service.manifest(command_id, raw)
    except CreationRequestError as error:
        raise _error(error) from error
    return JSONResponse(jsonable_encoder(value))
