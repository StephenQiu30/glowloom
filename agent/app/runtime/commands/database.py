import asyncio

from app.core.config import Settings, get_settings
from app.core.database import engine
from app.core.schema import assert_database_schema, initialize_database
from app.runtime.model_registry import register_implemented_models


async def prepare_database(settings: Settings) -> None:
    register_implemented_models()
    try:
        if settings.environment == "production":
            await assert_database_schema(engine)
        else:
            await initialize_database(engine)
    finally:
        await engine.dispose()


async def main() -> None:
    await prepare_database(get_settings())


if __name__ == "__main__":
    asyncio.run(main())
