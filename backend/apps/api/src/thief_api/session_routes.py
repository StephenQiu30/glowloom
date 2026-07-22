from __future__ import annotations

import hmac
from typing import Annotated, Protocol
from uuid import uuid4

from fastapi import APIRouter, Cookie, Header, Response
from fastapi.responses import JSONResponse
from pydantic import BaseModel

from thief_core.identity import (
    CreatedSession,
    InvalidCredentials,
    InvalidCsrf,
    InvalidSession,
    SessionIdentity,
)


SESSION_COOKIE = "thief_session"
CSRF_COOKIE = "thief_csrf"
COOKIE_MAX_AGE = 8 * 60 * 60


class SessionService(Protocol):
    def create(self, email: str, password: str) -> CreatedSession: ...

    def resolve(self, session_token: str) -> SessionIdentity: ...

    def revoke(self, session_token: str, csrf_token: str) -> None: ...


class SessionRequest(BaseModel):
    email: str
    password: str


class SessionView(BaseModel):
    user_id: str
    email: str
    role: str


def session_router(sessions: SessionService) -> APIRouter:
    router = APIRouter(prefix="/v1/session", tags=["session"])

    @router.post("", response_model=SessionView)
    def create_session(
        request: SessionRequest,
        response: Response,
    ) -> SessionView | JSONResponse:
        try:
            created = sessions.create(request.email, request.password)
        except (InvalidCredentials, ValueError):
            return _error(401, "invalid_credentials", "Email or password is invalid.")
        _set_cookies(response, created.session_token, created.csrf_token)
        return _view(created)

    @router.get("", response_model=SessionView)
    def get_session(
        session_token: Annotated[
            str | None,
            Cookie(alias=SESSION_COOKIE),
        ] = None,
    ) -> SessionView | JSONResponse:
        try:
            identity = sessions.resolve(session_token or "")
        except InvalidSession:
            return _error(401, "invalid_session", "Session is not valid.")
        return _view(identity)

    @router.delete("", status_code=204, response_model=None)
    def delete_session(
        response: Response,
        session_token: Annotated[
            str | None,
            Cookie(alias=SESSION_COOKIE),
        ] = None,
        csrf_cookie: Annotated[
            str | None,
            Cookie(alias=CSRF_COOKIE),
        ] = None,
        csrf_header: Annotated[
            str | None,
            Header(alias="X-CSRF-Token"),
        ] = None,
    ) -> Response | JSONResponse:
        if not csrf_cookie or not csrf_header or not hmac.compare_digest(
            csrf_cookie,
            csrf_header,
        ):
            return _error(403, "invalid_csrf", "CSRF validation failed.")
        try:
            sessions.revoke(session_token or "", csrf_header)
        except InvalidCsrf:
            return _error(403, "invalid_csrf", "CSRF validation failed.")
        except InvalidSession:
            return _error(401, "invalid_session", "Session is not valid.")
        response.status_code = 204
        _clear_cookies(response)
        return response

    return router


def _view(identity: SessionIdentity) -> SessionView:
    return SessionView(
        user_id=str(identity.user_id),
        email=identity.email,
        role=identity.role.value,
    )


def _set_cookies(response: Response, session_token: str, csrf_token: str) -> None:
    response.set_cookie(
        SESSION_COOKIE,
        session_token,
        max_age=COOKIE_MAX_AGE,
        secure=True,
        httponly=True,
        samesite="lax",
    )
    response.set_cookie(
        CSRF_COOKIE,
        csrf_token,
        max_age=COOKIE_MAX_AGE,
        secure=True,
        httponly=False,
        samesite="lax",
    )


def _clear_cookies(response: Response) -> None:
    response.delete_cookie(
        SESSION_COOKIE,
        secure=True,
        httponly=True,
        samesite="lax",
    )
    response.delete_cookie(
        CSRF_COOKIE,
        secure=True,
        httponly=False,
        samesite="lax",
    )


def _error(status: int, code: str, message: str) -> JSONResponse:
    return JSONResponse(
        status_code=status,
        content={
            "code": code,
            "message": message,
            "trace_id": uuid4().hex,
            "details": {},
        },
    )
