from __future__ import annotations

from uuid import uuid4

from fastapi.responses import JSONResponse


def error_response(status: int, code: str, message: str) -> JSONResponse:
    return JSONResponse(
        status_code=status,
        content={
            "code": code,
            "message": message,
            "trace_id": uuid4().hex,
            "details": {},
        },
    )
