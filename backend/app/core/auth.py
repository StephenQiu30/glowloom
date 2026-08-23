from datetime import UTC, datetime, timedelta
from typing import Annotated, Any, Literal
from uuid import UUID, uuid4

import jwt
from fastapi import Depends, Request
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from pydantic import BaseModel, ValidationError

from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode

ALGORITHM = "HS256"
_bearer = HTTPBearer(auto_error=False)


class AccessTokenClaims(BaseModel):
    sub: UUID
    ver: int
    type: Literal["access"]
    jti: str
    iss: str
    aud: str | list[str]
    iat: datetime
    exp: datetime


def create_access_token(user_id: UUID, token_version: int, settings: Settings) -> str:
    now = datetime.now(UTC)
    claims: dict[str, Any] = {
        "sub": str(user_id),
        "ver": token_version,
        "type": "access",
        "jti": uuid4().hex,
        "iss": settings.jwt_issuer,
        "aud": settings.jwt_audience,
        "iat": now,
        "exp": now + timedelta(minutes=settings.jwt_access_token_minutes),
    }
    return jwt.encode(  # pyright: ignore[reportUnknownMemberType]
        claims,
        settings.jwt_secret_key.get_secret_value(),
        algorithm=ALGORITHM,
    )


def decode_access_token(token: str, settings: Settings) -> AccessTokenClaims | None:
    try:
        claims = jwt.decode(  # pyright: ignore[reportUnknownMemberType]
            token,
            settings.jwt_secret_key.get_secret_value(),
            algorithms=[ALGORITHM],
            audience=settings.jwt_audience,
            issuer=settings.jwt_issuer,
            options={
                "require": [
                    "sub",
                    "ver",
                    "type",
                    "jti",
                    "iss",
                    "aud",
                    "iat",
                    "exp",
                ]
            },
        )
        return AccessTokenClaims.model_validate(claims)
    except (jwt.PyJWTError, ValidationError, ValueError):
        return None


def get_request_settings(request: Request) -> Settings:
    return request.app.state.settings


def get_access_token_claims(
    credentials: Annotated[HTTPAuthorizationCredentials | None, Depends(_bearer)],
    settings: Annotated[Settings, Depends(get_request_settings)],
) -> AccessTokenClaims:
    claims = (
        decode_access_token(credentials.credentials, settings)
        if credentials is not None and credentials.scheme.lower() == "bearer"
        else None
    )
    if claims is None:
        raise ApiError(
            ErrorCode.UNAUTHENTICATED,
            "Invalid credentials",
            status_code=401,
            next_action="login",
        )
    return claims
