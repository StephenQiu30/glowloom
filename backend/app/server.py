import asyncio
from collections.abc import Awaitable, Callable
from functools import partial

import uvicorn

from app.core.config import Settings, get_settings
from app.io_worker import run_io_worker
from app.media_worker import run_media_worker
from app.scheduler import run_scheduler

Service = Callable[[], Awaitable[None]]


async def supervise_services(*services: Service) -> None:
    tasks = {asyncio.ensure_future(service()) for service in services}
    try:
        done, pending = await asyncio.wait(tasks, return_when=asyncio.FIRST_COMPLETED)
        for task in pending:
            task.cancel()
        await asyncio.gather(*pending, return_exceptions=True)
        for task in done:
            task.result()
    finally:
        for task in tasks:
            if not task.done():
                task.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)


async def run_server(settings: Settings) -> None:
    api = uvicorn.Server(
        uvicorn.Config(
            "app.main:app",
            host="0.0.0.0",
            port=8000,
            access_log=False,
        )
    )
    await supervise_services(
        api.serve,
        partial(run_scheduler, settings),
        partial(run_io_worker, settings),
        partial(run_media_worker, settings),
    )


def main() -> None:
    asyncio.run(run_server(get_settings()))


if __name__ == "__main__":
    main()
