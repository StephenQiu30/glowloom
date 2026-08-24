import httpx


async def request_registration_ticket(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> str:
    requested = await client.post(
        "/api/v1/auth/registration-verifications",
        json={"email": email},
    )
    assert requested.status_code == 202, requested.text
    confirmed = await client.post(
        "/api/v1/auth/registration-verifications/confirm",
        json={"email": email, "code": "123456"},
    )
    assert confirmed.status_code == 200, confirmed.text
    return str(confirmed.json()["data"]["registration_ticket"])


async def register_identity_response(
    client: httpx.AsyncClient,
    *,
    email: str = "Creator@Example.com",
    password: str = "a-secure-test-password",
    display_name: str = "创作者",
) -> httpx.Response:
    ticket = await request_registration_ticket(client, email=email)
    return await client.post(
        "/api/v1/auth/register",
        json={
            "registration_ticket": ticket,
            "password": password,
            "display_name": display_name,
        },
    )
