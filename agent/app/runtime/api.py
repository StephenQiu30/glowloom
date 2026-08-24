import asyncio

import uvicorn

from app.core.config import Settings, get_settings
from app.core.logging import configure_logging
from app.core.schema import assert_database_schema
from app.core.telemetry import configure_telemetry
from app.runtime.model_registry import register_implemented_models


async def run_api(settings: Settings) -> None:
    register_implemented_models()
    await assert_database_schema()
    configure_logging(
        settings.log_level,
        service="lanverse-api",
        environment=settings.environment,
    )
    configure_telemetry(
        service_name="lanverse-api",
        environment=settings.environment,
    )
    server = uvicorn.Server(
        uvicorn.Config(
            "app.main:app",
            host=settings.api_host,
            port=settings.api_port,
            access_log=False,
        )
    )
    await server.serve()


def main() -> None:
    asyncio.run(run_api(get_settings()))


if __name__ == "__main__":
    main()
