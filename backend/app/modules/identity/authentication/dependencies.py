from typing import cast

from fastapi import Request

from app.modules.identity.authentication.contracts import AuthSessionStore


def get_auth_session_store(request: Request) -> AuthSessionStore:
    return cast(AuthSessionStore, request.app.state.auth_session_store)
