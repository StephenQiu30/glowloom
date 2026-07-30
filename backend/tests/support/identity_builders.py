import httpx


async def register_identity_response(
    client: httpx.AsyncClient,
    *,
    email: str = "Creator@Example.com",
    password: str = "a-secure-test-password",
    display_name: str = "创作者",
) -> httpx.Response:
    return await client.post(
        "/api/v1/auth/register",
        json={
            "email": email,
            "password": password,
            "display_name": display_name,
        },
    )
