"""Short-lived authorization for a frozen text invocation."""

import hashlib
import hmac

from app.protocol.canonical import canonical_hash
from app.text_contract.task import TextTask

_AUDIENCE = "lanverse.text-storyboard.invocation"


def sign_task(task: TextTask, secret: str, expires_at: int) -> str:
    if len(secret.encode()) < 32:
        raise ValueError("text task signing secret must contain at least 32 bytes")
    message = f"{_AUDIENCE}\n{expires_at}\n{canonical_hash(task.model_dump(mode='json'))}"
    signature = hmac.new(secret.encode(), message.encode(), hashlib.sha256).hexdigest()
    return f"{expires_at}.{signature}"


def verify_task(task: TextTask, token: str, secret: str, now: int) -> None:
    try:
        expires, signature = token.split(".", 1)
        expiry = int(expires)
        if str(expiry) != expires or not now < expiry <= now + 60:
            raise ValueError("invalid authorization window")
        expected = sign_task(task, secret, expiry)
        if len(signature) != 64 or not hmac.compare_digest(expected, token):
            raise ValueError("invalid authorization")
    except (ValueError, UnicodeError) as error:
        raise ValueError("invalid text task authorization") from error
