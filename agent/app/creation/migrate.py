"""Explicit migration entry: python -m app.creation.migrate."""

import asyncio

from app.creation.config import database_url
from app.creation.repository import Repository


async def migrate() -> None:
    await Repository(database_url()).migrate()


if __name__ == "__main__":
    asyncio.run(migrate())
