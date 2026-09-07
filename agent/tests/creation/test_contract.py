import base64
import hashlib
import hmac
import json
from pathlib import Path
from typing import Any

import pytest

from app.creation.authorization import InvalidAuthorization, verify_authorization
from app.creation.contract import Command, decode_object

SECRET = "creation-test-secret-at-least-thirty-two-bytes"


def command_payload() -> dict[str, Any]:
    path = Path(__file__).resolve().parents[3]
    return json.loads(
        (path / "backend/tests/production/creation/testdata/command.json").read_text()
    )["command"]


def authorization(
    body: bytes,
    path: str = "/internal/creation/commands",
    method: str = "POST",
    expires: int = 1060,
) -> str:
    claims = dict(
        audience="lanverse.creation.command",
        method=method,
        path=path,
        body_hash=hashlib.sha256(body).hexdigest(),
        expires_at=expires,
    )
    encoded = base64.urlsafe_b64encode(json.dumps(claims).encode()).rstrip(b"=")
    signature = base64.urlsafe_b64encode(hmac.digest(SECRET.encode(), encoded, "sha256")).rstrip(
        b"="
    )
    return (encoded + b"." + signature).decode()


def test_go_command_hash_and_strict_identity() -> None:
    command = Command.model_validate(command_payload())
    assert (
        command.payload_hash == "3a33c32aca8118992c4195676a1ed767cee4ab09ac80ca4ec737326570e49088"
    )
    for key, value in [
        ("run_id", "00000000-0000-0000-0000-000000000000"),
        ("workflow_id", "arbitrary"),
        ("extra", True),
        ("command_id", command.command_id.upper().replace("81", "AA", 1)),
    ]:
        with pytest.raises(ValueError):
            Command.model_validate({**command_payload(), key: value})
    for value in [True, "1", 1.0, 0, 2**63]:
        payload = command_payload()
        payload["source"]["revision"] = value
        with pytest.raises(ValueError):
            Command.model_validate(payload)


@pytest.mark.parametrize("body", [b'{"a":1,"a":2}', b"{} {}", b"[]", b'{"a":NaN}', b"\xff"])
def test_reject_ambiguous_json(body: bytes) -> None:
    with pytest.raises(ValueError):
        decode_object(body)


def test_hmac_binds_body_path_method_and_bounded_expiry() -> None:
    body = json.dumps(command_payload()).encode()
    token = authorization(body)
    verify_authorization(token, SECRET, "POST", "/internal/creation/commands", body, now=1000)
    for method, path, raw, now in [
        ("GET", "/internal/creation/commands", body, 1000),
        ("POST", "/wrong", body, 1000),
        ("POST", "/internal/creation/commands", body + b" ", 1000),
        ("POST", "/internal/creation/commands", body, 1060),
        ("POST", "/internal/creation/commands", body, 999),
    ]:
        with pytest.raises(InvalidAuthorization):
            verify_authorization(token, SECRET, method, path, raw, now=now)
    for invalid_token in ["", "bad.signature", "☃.signature", token + "="]:
        with pytest.raises(InvalidAuthorization):
            verify_authorization(
                invalid_token, SECRET, "POST", "/internal/creation/commands", body, now=1000
            )
