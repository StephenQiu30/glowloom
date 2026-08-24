import asyncio
import sys

from app.core.config import Settings, get_settings
from app.core.database import engine
from app.core.schema import adopt_database_baseline, assert_database_schema
from app.runtime.model_registry import register_implemented_models


async def prepare_database(settings: Settings) -> None:
    register_implemented_models()
    try:
        await assert_database_schema(engine)
    finally:
        await engine.dispose()


async def adopt_baseline() -> None:
    register_implemented_models()
    try:
        await adopt_database_baseline(engine)
    finally:
        await engine.dispose()


async def main() -> None:
    command = sys.argv[1] if len(sys.argv) > 1 else "check"
    if command == "check":
        await prepare_database(get_settings())
        return
    if command == "adopt-baseline":
        await adopt_baseline()
        return
    raise SystemExit(f"unknown database command: {command}")


if __name__ == "__main__":
    asyncio.run(main())
