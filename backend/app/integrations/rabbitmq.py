import aio_pika


async def rabbitmq_ping(url: str) -> None:
    connection = await aio_pika.connect_robust(url, timeout=1)
    try:
        channel = await connection.channel()
        try:
            await channel.declare_queue("lanverse.io", durable=True)
            await channel.declare_queue("lanverse.media", durable=True)
        finally:
            await channel.close()
    finally:
        await connection.close()
