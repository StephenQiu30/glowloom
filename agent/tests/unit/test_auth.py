from uuid import uuid4

import pytest
from pydantic import SecretStr, ValidationError

from app.core.auth import create_access_token, decode_access_token
from app.core.config import Settings


def _settings(*, audience: str = "lanverse-web") -> Settings:
    return Settings(
        environment="test",
        jwt_secret_key=SecretStr("unit-test-secret-with-at-least-32-bytes"),
        jwt_audience=audience,
    )


def test_access_token_round_trip_requires_expected_contract() -> None:
    user_id = uuid4()
    settings = _settings()

    token = create_access_token(user_id, 4, settings)
    claims = decode_access_token(token, settings)

    assert claims is not None
    assert claims.sub == user_id
    assert claims.ver == 4
    assert claims.type == "access"


def test_access_token_rejects_wrong_audience_and_tampering() -> None:
    settings = _settings()
    token = create_access_token(uuid4(), 1, settings)
    header, payload, signature = token.split(".")
    replacement = "A" if signature[0] != "A" else "B"
    tampered_token = f"{header}.{payload}.{replacement}{signature[1:]}"

    assert decode_access_token(token, _settings(audience="another-client")) is None
    assert decode_access_token(tampered_token, settings) is None


def test_production_rejects_the_committed_development_secret() -> None:
    with pytest.raises(ValidationError, match="JWT_SECRET_KEY must be set"):
        Settings(
            environment="production",
            jwt_secret_key=SecretStr("development-only-jwt-secret-change-before-production"),
        )
