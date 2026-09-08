"""Signed internal access to frozen execution plans."""

from collections.abc import Awaitable, Callable

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse

from app.creation.contract import canonical_uuid
from app.creation.manifest import ManifestConflict, read_manifest
from app.creation.repository import Repository


def install_manifest_routes(
    app: FastAPI,
    repository: Repository,
    authorized_body: Callable[[Request], Awaitable[bytes]],
) -> None:
    async def manifest(command_id: str, request: Request) -> JSONResponse:
        raw = await authorized_body(request)
        try:
            canonical_uuid(command_id)
            if raw:
                raise ValueError("manifest lookup body must be empty")
        except ValueError as error:
            raise HTTPException(422, "creation_manifest_lookup_invalid") from error
        try:
            async with await repository.connect() as conn:
                saved = await read_manifest(conn, command_id)
        except ManifestConflict as error:
            raise HTTPException(409, "creation_manifest_conflict") from error
        if saved is None:
            raise HTTPException(404, "creation_command_not_found")
        return JSONResponse(saved)

    app.add_api_route(
        "/internal/creation/commands/{command_id}/manifest", manifest, methods=["GET"]
    )
