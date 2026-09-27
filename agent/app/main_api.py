"""agent-api: internal HTTP entry of the Agent service (OPS-01 §3, port 8090)."""

import logging
import re
from collections.abc import Awaitable, Callable, Mapping
from uuid import uuid4

from fastapi import FastAPI, Request
from fastapi.exceptions import RequestValidationError
from fastapi.responses import JSONResponse, Response
from starlette.exceptions import HTTPException

from app.config import Settings

_REQUEST_ID = re.compile(r"[A-Za-z0-9._-]{1,128}\Z")
_HTTP_ERRORS = {
    400: ("validation_failed", "Invalid request"),
    401: ("unauthenticated", "Unauthenticated"),
    403: ("forbidden", "Forbidden"),
    404: ("not_found", "Not found"),
    405: ("method_not_allowed", "Method not allowed"),
    500: ("internal", "Internal server error"),
    503: ("dependency_unavailable", "Dependency unavailable"),
}
_logger = logging.getLogger(__name__)


def _request_id(request: Request) -> str:
    value = request.headers.get("x-request-id", "")
    return value if _REQUEST_ID.fullmatch(value) else uuid4().hex


def _problem(
    request: Request,
    status: int,
    code: str,
    title: str,
    *,
    headers: Mapping[str, str] | None = None,
) -> JSONResponse:
    request_id: str = request.state.request_id
    return JSONResponse(
        status_code=status,
        media_type="application/problem+json",
        headers={**(headers or {}), "X-Request-Id": request_id},
        content={
            "type": f"https://lanverse.local/errors/{code}",
            "title": title,
            "status": status,
            "code": code,
            "request_id": request_id,
        },
    )


def create_app(settings: Settings | None = None) -> FastAPI:
    settings = settings or Settings()
    # Internal-only service: no public OpenAPI docs.
    app = FastAPI(title="lanverse-agent", docs_url=None, redoc_url=None, openapi_url=None)
    app.state.settings = settings

    @app.middleware("http")
    async def request_identity(
        request: Request, call_next: Callable[[Request], Awaitable[Response]]
    ) -> Response:
        request.state.request_id = _request_id(request)
        response = await call_next(request)
        response.headers["X-Request-Id"] = request.state.request_id
        return response

    @app.exception_handler(RequestValidationError)
    async def validation_error(request: Request, _error: RequestValidationError) -> JSONResponse:
        return _problem(request, 400, *_HTTP_ERRORS[400])

    @app.exception_handler(HTTPException)
    async def http_error(request: Request, error: HTTPException) -> JSONResponse:
        code, title = _HTTP_ERRORS.get(error.status_code, ("http_error", "HTTP error"))
        return _problem(request, error.status_code, code, title, headers=error.headers)

    @app.exception_handler(Exception)
    async def unexpected_error(request: Request, error: Exception) -> JSONResponse:
        _logger.error(
            "agent_api_unexpected_error",
            extra={"request_id": request.state.request_id, "error_type": type(error).__name__},
        )
        return _problem(request, 500, *_HTTP_ERRORS[500])

    @app.get("/internal/health")
    def health() -> dict[str, str]:
        return {"status": "ok"}

    return app
