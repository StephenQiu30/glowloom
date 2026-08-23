import pytest

from app.cache_maintenance import invalidate_cache_namespace
from app.core.config import Settings
from app.modules.caching import CacheKey, CacheNamespace


class NamespaceRecordingCache:
    def __init__(self) -> None:
        self.invalidated: list[CacheNamespace] = []

    async def get(self, key: CacheKey) -> bytes | None:
        raise AssertionError("get is not part of namespace invalidation")

    async def set(
        self,
        key: CacheKey,
        value: bytes,
        *,
        ttl_seconds: int,
        jitter_seconds: int,
    ) -> None:
        raise AssertionError("set is not part of namespace invalidation")

    async def set_revision(
        self,
        key: CacheKey,
        revision: int,
        *,
        ttl_seconds: int,
        jitter_seconds: int,
    ) -> bool:
        raise AssertionError("set_revision is not part of namespace invalidation")

    async def delete(self, key: CacheKey) -> None:
        raise AssertionError("delete is not part of namespace invalidation")

    async def invalidate_namespace(self, namespace: CacheNamespace) -> int:
        self.invalidated.append(namespace)
        return 3

    async def close(self) -> None:
        raise AssertionError("an injected cache is owned by the caller")


@pytest.mark.asyncio
async def test_cache_maintenance_only_invalidates_a_registered_namespace() -> None:
    cache = NamespaceRecordingCache()

    removed = await invalidate_cache_namespace(
        Settings(),
        CacheNamespace.WORKSPACE_DETAIL,
        cache_port=cache,
    )

    assert removed == 3
    assert cache.invalidated == [CacheNamespace.WORKSPACE_DETAIL]
    with pytest.raises(ValueError):
        CacheNamespace("unregistered")
