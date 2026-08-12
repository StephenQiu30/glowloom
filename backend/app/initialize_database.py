import asyncio

from app.core.database import engine
from app.core.migrations import upgrade_database
from app.model_registry import register_implemented_models


async def main() -> None:
    register_implemented_models()
    await upgrade_database(engine)
    await engine.dispose()


if __name__ == "__main__":
    asyncio.run(main())
