"""Infrastructure-free application factory used by Agent HTTP tests."""

from __future__ import annotations

from typing import cast

from fastapi import FastAPI

from app.creation.repository import Repository
from app.main import create_app


class NoopRepository:
    async def ready(self) -> None:
        return None


def create_test_app() -> FastAPI:
    """Build the dependency-injected app used by HTTP and browser tests."""

    return create_app(
        repository=cast(Repository, NoopRepository()),
        secret="synthetic-creation-secret-for-tests-at-least-32-bytes",
        task_queue="test",
    )


test_app = create_test_app()
