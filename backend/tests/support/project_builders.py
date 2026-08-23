import httpx

from tests.support.identity_builders import register_identity_response


async def register_project_owner(
    client: httpx.AsyncClient,
    *,
    email: str = "project-owner@example.com",
) -> tuple[dict[str, str], str]:
    response = await register_identity_response(
        client,
        email=email,
        password="a-secure-project-password",
        display_name="项目负责人",
    )
    assert response.status_code == 201
    data = response.json()["data"]
    return {"authorization": f"Bearer {data['access_token']}"}, data["workspace"]["id"]


def project_payload(workspace_id: str, name: str = "竖屏短剧") -> dict[str, object]:
    return {
        "workspace_id": workspace_id,
        "name": name,
        "description": "一部用于验收的短剧",
        "aspect_ratio": "9:16",
        "language": "zh-CN",
        "visual_style": "写实电影感",
        "target_duration_ms": 90000,
    }
