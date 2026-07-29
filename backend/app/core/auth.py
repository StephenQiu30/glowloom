from datetime import UTC, datetime, timedelta
from typing import Any, Literal
from uuid import UUID

import jwt
from pydantic import BaseModel, ValidationError

from app.core.config import Settings

ALGORITHM = "HS256"


class AccessTokenClaims(BaseModel):
    sub: UUID
    ver: int
    type: Literal["access"]
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
            options={"require": ["sub", "ver", "type", "iss", "aud", "iat", "exp"]},
        )
        return AccessTokenClaims.model_validate(claims)
    except (jwt.PyJWTError, ValidationError, ValueError):
        return None
