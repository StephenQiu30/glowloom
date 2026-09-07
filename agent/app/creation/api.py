from __future__ import annotations

import asyncio
from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager, suppress

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse
from psycopg import Error as DatabaseError
from temporalio.client import Client

from app.creation.authorization import HEADER, InvalidAuthorization, verify_authorization
from app.creation.config import Settings
from app.creation.contract import Command, canonical_uuid, decode_object
from app.creation.dispatcher import Dispatcher
from app.creation.repository import CommandConflict, Repository, SchemaMismatch
from app.creation.temporal import TemporalStarter

MAX_COMMAND_BYTES = 16384


def create_app(repository: Repository, secret: str, task_queue: str) -> FastAPI:
    app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None)

    async def authorized_body(request: Request) -> bytes:
        raw = bytearray()
        async for chunk in request.stream():
            if len(raw) + len(chunk) > MAX_COMMAND_BYTES:
                raise HTTPException(413, "creation_command_too_large")
            raw.extend(chunk)
        try:
            tokens = request.headers.getlist(HEADER)
            if len(tokens) != 1 or request.url.query:
                raise InvalidAuthorization("missing or ambiguous authorization")
            path = request.scope["raw_path"].decode("ascii")
            verify_authorization(tokens[0], secret, request.method, path, bytes(raw))
        except (InvalidAuthorization, UnicodeError) as error:
            raise HTTPException(401, "creation_authorization_invalid") from error
        return bytes(raw)

    async def accept(request: Request) -> JSONResponse:
        raw = await authorized_body(request)
        try:
            command = Command.model_validate(decode_object(raw))
        except ValueError as error:
            raise HTTPException(422, "creation_command_invalid") from error
        try:
            receipt = await repository.accept(command, task_queue)
        except CommandConflict as error:
            raise HTTPException(409, "creation_command_conflict") from error
        return JSONResponse(receipt.model_dump(mode="json", by_alias=True), status_code=202)

    async def lookup(command_id: str, request: Request) -> JSONResponse:
        raw = await authorized_body(request)
        try:
            canonical_uuid(command_id)
            if raw:
                raise ValueError("lookup body must be empty")
        except ValueError as error:
            raise HTTPException(422, "creation_command_invalid") from error
        receipt = await repository.get(command_id)
        if receipt is None:
            raise HTTPException(404, "creation_command_not_found")
        return JSONResponse(receipt.model_dump(mode="json", by_alias=True))

    async def health() -> dict[str, str]:
        return {"status": "ok"}

    async def ready() -> dict[str, str]:
        await repository.ready()
        return {"status": "ready", "capability": "durable-command-acceptance"}

    async def unavailable(request: Request, error: Exception) -> JSONResponse:
        return JSONResponse({"detail": "creation_storage_unavailable"}, status_code=503)

    app.add_api_route("/internal/creation/commands", accept, methods=["POST"])
    app.add_api_route("/internal/creation/commands/{command_id}", lookup, methods=["GET"])
    app.add_api_route("/healthz", health, methods=["GET"])
    app.add_api_route("/readyz", ready, methods=["GET"])
    app.add_exception_handler(DatabaseError, unavailable)
    app.add_exception_handler(SchemaMismatch, unavailable)
    return app


def create_configured_app() -> FastAPI:
    settings = Settings.from_environment()
    repository = Repository(settings.database_url)
    app = create_app(repository, settings.secret, settings.task_queue)

    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncGenerator[None]:
        await repository.ready()
        client = await Client.connect(
            settings.temporal_address,
            namespace=settings.temporal_namespace,
            tls=settings.temporal_tls,
            lazy=True,
        )
        dispatcher = Dispatcher(repository, TemporalStarter(client))
        task = asyncio.create_task(dispatcher.run(), name="creation-start-dispatcher")
        try:
            yield
        finally:
            task.cancel()
            with suppress(asyncio.CancelledError):
                await task

    app.router.lifespan_context = lifespan
    return app
