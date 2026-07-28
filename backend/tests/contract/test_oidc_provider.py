from __future__ import annotations

import os
import secrets
from collections.abc import Generator
from contextlib import contextmanager
from typing import Any, cast

import httpx
import jwt


def _require_contract_environment() -> tuple[str, str, str]:
    if os.getenv("LANVERSE_RUN_OIDC_CONTRACT") != "1":
        import pytest

        pytest.skip("set LANVERSE_RUN_OIDC_CONTRACT=1 with the identity profile running")

    base_url = os.getenv("LANVERSE_OIDC_BASE_URL", "http://127.0.0.1:8080").rstrip("/")
    admin_username = os.environ["KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME"]
    admin_password = os.environ["KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD"]
    return base_url, admin_username, admin_password


def _json(response: httpx.Response) -> dict[str, Any]:
    response.raise_for_status()
    return cast(dict[str, Any], response.json())


@contextmanager
def _test_user(
    client: httpx.Client,
    *,
    base_url: str,
    admin_token: str,
) -> Generator[tuple[str, str]]:
    username = f"contract-{secrets.token_hex(8)}"
    password = secrets.token_urlsafe(24)
    headers = {"Authorization": f"Bearer {admin_token}"}
    response = client.post(
        f"{base_url}/admin/realms/lanverse-test/users",
        headers=headers,
        json={"username": username, "enabled": True},
    )
    response.raise_for_status()
    user_id = response.headers["Location"].rsplit("/", maxsplit=1)[-1]
    client.put(
        f"{base_url}/admin/realms/lanverse-test/users/{user_id}/reset-password",
        headers=headers,
        json={"type": "password", "temporary": False, "value": password},
    ).raise_for_status()
    try:
        yield username, password
    finally:
        client.delete(
            f"{base_url}/admin/realms/lanverse-test/users/{user_id}",
            headers=headers,
        ).raise_for_status()


def test_keycloak_issues_a_verifiable_lanverse_access_token() -> None:
    base_url, admin_username, admin_password = _require_contract_environment()
    issuer = f"{base_url}/realms/lanverse-test"

    with httpx.Client(timeout=10) as client:
        discovery = _json(client.get(f"{issuer}/.well-known/openid-configuration"))
        assert discovery["issuer"] == issuer
        assert discovery["end_session_endpoint"].endswith("/protocol/openid-connect/logout")

        admin_token = _json(
            client.post(
                f"{base_url}/realms/master/protocol/openid-connect/token",
                data={
                    "grant_type": "password",
                    "client_id": "admin-cli",
                    "username": admin_username,
                    "password": admin_password,
                },
            )
        )["access_token"]

        with _test_user(client, base_url=base_url, admin_token=admin_token) as credentials:
            access_token = _json(
                client.post(
                    str(discovery["token_endpoint"]),
                    data={
                        "grant_type": "password",
                        "client_id": "lanverse-contract",
                        "username": credentials[0],
                        "password": credentials[1],
                    },
                )
            )["access_token"]

    signing_key = jwt.PyJWKClient(str(discovery["jwks_uri"])).get_signing_key_from_jwt(
        access_token
    )
    claims = jwt.decode(
        access_token,
        signing_key.key,
        algorithms=["RS256"],
        audience="lanverse-api",
        issuer=issuer,
        options={"require": ["aud", "exp", "iss", "sub"]},
    )
    assert claims["preferred_username"].startswith("contract-")
