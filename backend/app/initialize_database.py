import asyncio

from app.core.database import engine, initialize_empty_database
from app.model_registry import register_implemented_models


async def main() -> None:
    register_implemented_models()
    await initialize_empty_database()
    await engine.dispose()


if __name__ == "__main__":
    asyncio.run(main())
