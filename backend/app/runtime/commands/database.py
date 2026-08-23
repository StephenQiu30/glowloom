import asyncio

from app.core.database import engine
from app.core.schema import initialize_database
from app.runtime.model_registry import register_implemented_models


async def main() -> None:
    register_implemented_models()
    await initialize_database(engine)
    await engine.dispose()


if __name__ == "__main__":
    asyncio.run(main())
