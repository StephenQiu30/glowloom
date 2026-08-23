import argparse
import asyncio

from app.core.config import Settings, get_settings
from app.integrations.redis import RedisCache
from app.modules.caching import CacheNamespace, CachePort


async def invalidate_cache_namespace(
    settings: Settings,
    namespace: CacheNamespace,
    *,
    cache_port: CachePort | None = None,
) -> int:
    cache = cache_port or RedisCache(
        settings.redis_url,
        environment=settings.environment,
        socket_timeout_seconds=min(settings.infrastructure_timeout_seconds, 0.25),
    )
    owns_cache = cache_port is None
    try:
        return await cache.invalidate_namespace(namespace)
    finally:
        if owns_cache:
            await cache.close()


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Invalidate one registered Lanverse cache namespace"
    )
    parser.add_argument(
        "namespace",
        choices=[namespace.value for namespace in CacheNamespace],
    )
    arguments = parser.parse_args()
    namespace = CacheNamespace(arguments.namespace)
    removed = asyncio.run(invalidate_cache_namespace(get_settings(), namespace))
    print(f"invalidated namespace={namespace.value} keys={removed}")


if __name__ == "__main__":
    main()
