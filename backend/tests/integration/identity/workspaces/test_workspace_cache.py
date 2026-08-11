from collections.abc import AsyncIterator

import httpx
import pytest

from app.modules.caching.contracts import (
    CacheKey,
    CacheNamespace,
    CachePort,
    CacheUnavailableError,
)
from tests.support.identity_builders import register_identity_response


class RecordingCache:
    def __init__(self) -> None:
        self.values: dict[CacheKey, bytes] = {}
        self.get_calls: list[CacheKey] = []
        self.set_calls: list[CacheKey] = []
        self.delete_calls: list[CacheKey] = []
        self.fail_get = False
        self.fail_delete = False

    async def get(self, key: CacheKey) -> bytes | None:
        self.get_calls.append(key)
        if self.fail_get:
            raise CacheUnavailableError("injected get failure")
        return self.values.get(key)

    async def set(
        self,
        key: CacheKey,
        value: bytes,
        *,
        ttl_seconds: int,
        jitter_seconds: int,
    ) -> None:
        assert ttl_seconds > 0
        assert 0 <= jitter_seconds < ttl_seconds
        self.set_calls.append(key)
        self.values[key] = value

    async def delete(self, key: CacheKey) -> None:
        self.delete_calls.append(key)
        if self.fail_delete:
            raise CacheUnavailableError("injected delete failure")
        self.values.pop(key, None)

    async def set_revision(
        self,
        key: CacheKey,
        revision: int,
        *,
        ttl_seconds: int,
        jitter_seconds: int,
    ) -> bool:
        assert ttl_seconds > 0
        assert 0 <= jitter_seconds < ttl_seconds
        current = self.values.get(key)
        if current is not None and int(current) > revision:
            return False
        self.set_calls.append(key)
        self.values[key] = str(revision).encode("ascii")
        return True

    async def invalidate_namespace(self, namespace: CacheNamespace) -> int:
        matching = [key for key in self.values if key.namespace == namespace]
        for key in matching:
            self.values.pop(key)
        return len(matching)

    async def close(self) -> None:
        return None


@pytest.fixture
async def cache_port() -> AsyncIterator[CachePort]:
    yield RecordingCache()


@pytest.mark.asyncio
async def test_workspace_detail_is_authorized_then_cached_and_invalidated_after_write(
    client: httpx.AsyncClient,
    cache_port: CachePort,
) -> None:
    cache = cache_port
    assert isinstance(cache, RecordingCache)
    registered = await register_identity_response(client)
    data = registered.json()["data"]
    workspace_id = data["workspace"]["id"]
    headers = {"authorization": f"Bearer {data['access_token']}"}

    cold = await client.get(f"/api/v1/workspaces/{workspace_id}", headers=headers)
    assert cold.status_code == 200
    assert cold.json()["data"]["revision"] == 1
    assert len(cache.set_calls) == 2

    hit = await client.get(f"/api/v1/workspaces/{workspace_id}", headers=headers)
    assert hit.status_code == 200
    assert hit.json() == cold.json()
    assert len(cache.set_calls) == 2
    assert [key.identity for key in cache.get_calls[-2:]] == ["revision", "workspace"]

    renamed = await client.patch(
        f"/api/v1/workspaces/{workspace_id}",
        headers=headers,
        json={"name": "缓存失效后的空间", "expected_revision": 1},
    )
    assert renamed.status_code == 200
    assert renamed.json()["data"]["revision"] == 2
    assert cache.delete_calls[-1].identity == "revision"

    refreshed = await client.get(f"/api/v1/workspaces/{workspace_id}", headers=headers)
    assert refreshed.status_code == 200
    assert refreshed.json()["data"]["name"] == "缓存失效后的空间"
    assert refreshed.json()["data"]["revision"] == 2


@pytest.mark.asyncio
async def test_cache_is_never_read_before_membership_and_failures_do_not_change_facts(
    client: httpx.AsyncClient,
    cache_port: CachePort,
) -> None:
    cache = cache_port
    assert isinstance(cache, RecordingCache)
    registered = await register_identity_response(client)
    data = registered.json()["data"]
    workspace_id = data["workspace"]["id"]
    headers = {"authorization": f"Bearer {data['access_token']}"}
    await client.get(f"/api/v1/workspaces/{workspace_id}", headers=headers)

    other = await register_identity_response(
        client,
        email="workspace-cache-other@example.com",
        password="another-secure-password",
        display_name="缓存隔离用户",
    )
    other_headers = {
        "authorization": f"Bearer {other.json()['data']['access_token']}"
    }
    cache.get_calls.clear()
    hidden = await client.get(
        f"/api/v1/workspaces/{workspace_id}", headers=other_headers
    )
    assert hidden.status_code == 404
    assert cache.get_calls == []

    cache.fail_delete = True
    renamed = await client.patch(
        f"/api/v1/workspaces/{workspace_id}",
        headers=headers,
        json={"name": "Redis 故障仍提交", "expected_revision": 1},
    )
    assert renamed.status_code == 200
    assert renamed.json()["data"]["name"] == "Redis 故障仍提交"
    cache.fail_get = True
    recovered = await client.get(
        f"/api/v1/workspaces/{workspace_id}", headers=headers
    )
    assert recovered.status_code == 200
    assert recovered.json()["data"]["name"] == "Redis 故障仍提交"
    assert recovered.json()["data"]["revision"] == 2
