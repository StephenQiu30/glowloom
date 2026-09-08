from __future__ import annotations

import asyncio
from collections.abc import AsyncGenerator
from contextlib import asynccontextmanager, suppress
from datetime import timedelta

from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import JSONResponse
from psycopg import Error as DatabaseError
from temporalio.client import Client
from temporalio.service import RPCError

from app.creation.attempt_api import install_attempt_routes
from app.creation.authorization import HEADER, InvalidAuthorization, verify_authorization
from app.creation.config import Settings
from app.creation.contract import Command, canonical_uuid, decode_object
from app.creation.dispatcher import Dispatcher
from app.creation.execution import ExecutionConflict, ExecutionStore
from app.creation.manifest_api import install_manifest_routes
from app.creation.repository import CommandConflict, Repository, SchemaMismatch
from app.creation.temporal import TemporalStarter

MAX_COMMAND_BYTES = 16384


def create_app(
    repository: Repository, secret: str, task_queue: str, temporal: Client | None = None
) -> FastAPI:
    app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None)
    execution = ExecutionStore(repository)
    app.state.temporal = temporal

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

    async def execution_identity(command_id: str, request: Request) -> dict[str, str]:
        raw = await authorized_body(request)
        try:
            canonical_uuid(command_id)
            if raw:
                raise ValueError("lookup body must be empty")
        except ValueError as error:
            raise HTTPException(422, "creation_command_invalid") from error
        command = await repository.command(command_id)
        if command is None:
            raise HTTPException(404, "creation_command_not_found")
        return {
            "command_id": command_id,
            "run_id": command.run_id,
            "payload_hash": command.payload_hash,
            "source_revision_id": command.source.revision_id,
        }

    async def snapshot(command_id: str, request: Request) -> JSONResponse:
        identity = await execution_identity(command_id, request)
        try:
            saved = await execution.snapshot(command_id)
        except ExecutionConflict as error:
            if str(error) == "execution_policy_missing":
                raise HTTPException(404, "creation_execution_not_started") from error
            raise
        return JSONResponse({"schema": "creation-execution-production", **identity, **saved})

    async def draft(command_id: str, draft_id: str, request: Request) -> JSONResponse:
        identity = await execution_identity(command_id, request)
        try:
            canonical_uuid(draft_id)
        except ValueError as error:
            raise HTTPException(422, "creation_draft_invalid") from error
        saved = await execution.draft_envelope(command_id, draft_id)
        if saved is None:
            raise HTTPException(404, "creation_draft_not_found")
        return JSONResponse({"schema": "creation-draft-production", **identity, **saved})

    async def conflict(request: Request, error: Exception) -> JSONResponse:
        return JSONResponse({"detail": "creation_execution_conflict"}, status_code=409)

    async def resume(command_id: str, request: Request) -> JSONResponse:
        raw = await authorized_body(request)
        try:
            canonical_uuid(command_id)
            payload = decode_object(raw)
            if set(payload) != {"payload_hash"}:
                raise ValueError("invalid resume input")
        except ValueError as error:
            raise HTTPException(422, "creation_resume_invalid") from error
        command = await repository.command(command_id)
        if command is None:
            raise HTTPException(404, "creation_command_not_found")
        if payload["payload_hash"] != command.payload_hash:
            raise HTTPException(409, "creation_command_conflict")
        saved = await execution.snapshot(command_id)
        if saved["status"] in {"running", "waiting_review", "completed"}:
            return JSONResponse(
                {
                    "schema": "creation-resume-production",
                    "command_id": command_id,
                    "run_id": command.run_id,
                    "status": "resume_requested",
                },
                status_code=202,
            )
        if saved["status"] != "blocked" or not saved["can_resume"]:
            raise HTTPException(409, "creation_resume_not_allowed")
        client: Client | None = app.state.temporal
        if client is None:
            raise HTTPException(503, "creation_runtime_unavailable")
        handle = client.get_workflow_handle(command.workflow_id)
        try:
            description = await handle.describe(rpc_timeout=timedelta(seconds=5))
            if description.workflow_type != command.flow_type:
                raise HTTPException(409, "creation_workflow_identity_conflict")
            for key, value in TemporalStarter.memo(command).items():
                if await description.memo_value(key, default=None) != value:
                    raise HTTPException(409, "creation_workflow_identity_conflict")
            await handle.signal("resume", rpc_timeout=timedelta(seconds=5))
        except RPCError as error:
            raise HTTPException(503, "creation_runtime_unavailable") from error
        return JSONResponse(
            {
                "schema": "creation-resume-production",
                "command_id": command_id,
                "run_id": command.run_id,
                "status": "resume_requested",
            },
            status_code=202,
        )

    async def ready() -> dict[str, str]:
        await repository.ready()
        return {"status": "ready", "capability": "durable-command-acceptance"}

    async def unavailable(request: Request, error: Exception) -> JSONResponse:
        return JSONResponse({"detail": "creation_storage_unavailable"}, status_code=503)

    app.add_api_route("/internal/creation/commands", accept, methods=["POST"])
    app.add_api_route("/internal/creation/commands/{command_id}", lookup, methods=["GET"])
    app.add_api_route("/internal/creation/commands/{command_id}/resume", resume, methods=["POST"])
    app.add_api_route(
        "/internal/creation/commands/{command_id}/execution", snapshot, methods=["GET"]
    )
    app.add_api_route(
        "/internal/creation/commands/{command_id}/drafts/{draft_id}", draft, methods=["GET"]
    )
    app.add_api_route("/healthz", health, methods=["GET"])
    app.add_api_route("/readyz", ready, methods=["GET"])
    app.add_exception_handler(DatabaseError, unavailable)
    app.add_exception_handler(SchemaMismatch, unavailable)
    app.add_exception_handler(ExecutionConflict, conflict)
    install_attempt_routes(app, repository, authorized_body)
    install_manifest_routes(app, repository, authorized_body)
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
        app.state.temporal = client
        task = asyncio.create_task(dispatcher.run(), name="creation-start-dispatcher")
        try:
            yield
        finally:
            task.cancel()
            with suppress(asyncio.CancelledError):
                await task

    app.router.lifespan_context = lifespan
    return app
