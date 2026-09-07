from __future__ import annotations

import base64
import hashlib
import hmac
import re
import time
from typing import Literal

from app.creation.contract import ContentHash, StrictModel, decode_object

HEADER = "X-Lanverse-Creation-Authorization"


class InvalidAuthorization(ValueError):
    pass


class Claims(StrictModel):
    audience: Literal["lanverse.creation.command"]
    method: Literal["GET", "POST"]
    path: str
    body_hash: ContentHash
    expires_at: int


def verify_authorization(
    token: str, secret: str, method: str, path: str, body: bytes, *, now: int | None = None
) -> None:
    try:
        if len(secret.encode()) < 32 or len(token) > 4096:
            raise ValueError("invalid authorization configuration or size")
        encoded, signature = token.split(".")
        if not re.fullmatch(r"[A-Za-z0-9_-]+", encoded):
            raise ValueError("invalid authorization encoding")
        expected = (
            base64.urlsafe_b64encode(
                hmac.digest(secret.encode(), encoded.encode("ascii"), "sha256")
            )
            .rstrip(b"=")
            .decode("ascii")
        )
        if not hmac.compare_digest(signature.encode("utf-8"), expected.encode("ascii")):
            raise ValueError("invalid signature")
        claims = Claims.model_validate(
            decode_object(base64.urlsafe_b64decode(encoded + "=" * (-len(encoded) % 4)))
        )
        instant = int(time.time()) if now is None else now
        if not instant < claims.expires_at <= instant + 60:
            raise ValueError("invalid expiry")
        if (
            claims.method != method
            or claims.path != path
            or claims.body_hash != hashlib.sha256(body).hexdigest()
        ):
            raise ValueError("request does not match authorization")
    except (ValueError, UnicodeError) as error:
        raise InvalidAuthorization("invalid creation authorization") from error
