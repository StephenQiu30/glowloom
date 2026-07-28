import asyncio

from app.core.database import engine, initialize_empty_database


async def main() -> None:
    await initialize_empty_database()
    await engine.dispose()


if __name__ == "__main__":
    asyncio.run(main())
