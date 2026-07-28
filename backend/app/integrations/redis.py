from redis.asyncio import Redis


async def redis_ping(url: str) -> None:
    # redis-py leaves connection keyword arguments untyped in its public stub.
    client = Redis.from_url(  # pyright: ignore[reportUnknownMemberType]
        url,
        socket_connect_timeout=1,
        socket_timeout=1,
    )
    try:
        # The ping return is bool at runtime but its stub exposes unknown kwargs.
        if not await client.ping():  # pyright: ignore[reportUnknownMemberType]
            raise ConnectionError("redis ping returned false")
    finally:
        await client.aclose()
