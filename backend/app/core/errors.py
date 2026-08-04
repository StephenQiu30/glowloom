from enum import StrEnum
from typing import Any

from fastapi import FastAPI, Request
from fastapi.responses import JSONResponse


class ErrorCode(StrEnum):
    INVALID_REQUEST = "invalid_request"
    VALIDATION_FAILED = "validation_failed"
    UNAUTHENTICATED = "unauthenticated"
    FORBIDDEN = "forbidden"
    NOT_FOUND = "not_found"
    RESOURCE_CONFLICT = "resource_conflict"
    VERSION_CONFLICT = "version_conflict"
    STATE_CONFLICT = "state_conflict"
    QUOTA_INSUFFICIENT = "quota_insufficient"
    DEPENDENCY_UNAVAILABLE = "dependency_unavailable"
    INTERNAL_ERROR = "internal_error"


class ApiError(Exception):
    def __init__(
        self,
        code: ErrorCode,
        message: str,
        *,
        status_code: int = 400,
        next_action: str | None = None,
        details: dict[str, Any] | None = None,
    ) -> None:
        super().__init__(message)
        self.code = code
        self.message = message
        self.status_code = status_code
        self.next_action = next_action
        self.details = details or {}


def register_exception_handlers(app: FastAPI) -> None:
    @app.exception_handler(ApiError)
    # FastAPI registers this function through the decorator at runtime.
    async def _handle_api_error(  # pyright: ignore[reportUnusedFunction]
        request: Request, error: ApiError
    ) -> JSONResponse:
        return JSONResponse(
            status_code=error.status_code,
            content={
                "error": {
                    "code": error.code,
                    "message": error.message,
                    "request_id": getattr(request.state, "request_id", None),
                    "next_action": error.next_action,
                    "details": error.details,
                }
            },
        )
