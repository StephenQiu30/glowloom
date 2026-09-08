"""Authenticated read-only access to the trusted execution journal."""

from collections.abc import Awaitable, Callable

from fastapi import FastAPI, HTTPException, Request
from fastapi.encoders import jsonable_encoder
from fastapi.responses import JSONResponse

from app.creation.contract import canonical_uuid
from app.creation.execution import ExecutionStore
from app.creation.repository import Repository


def install_attempt_routes(
    app: FastAPI,
    repository: Repository,
    authorized_body: Callable[[Request], Awaitable[bytes]],
) -> None:
    execution = ExecutionStore(repository)

    async def attempts(command_id: str, step_id: str, request: Request) -> JSONResponse:
        raw = await authorized_body(request)
        try:
            canonical_uuid(command_id)
            canonical_uuid(step_id)
            if raw:
                raise ValueError("attempt lookup body must be empty")
        except ValueError as error:
            raise HTTPException(422, "creation_attempt_lookup_invalid") from error
        command = await repository.command(command_id)
        if command is None:
            raise HTTPException(404, "creation_command_not_found")
        history = await execution.attempt_history(command_id, step_id)
        if history is None:
            raise HTTPException(404, "creation_step_not_found")
        return JSONResponse(
            jsonable_encoder(
                {
                    "schema": "creation-attempt-history-production",
                    "command_id": command_id,
                    "run_id": command.run_id,
                    **history,
                }
            )
        )

    app.add_api_route(
        "/internal/creation/commands/{command_id}/steps/{step_id}/attempts",
        attempts,
        methods=["GET"],
    )
