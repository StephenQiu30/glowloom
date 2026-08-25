from collections.abc import AsyncIterator

import httpx
import pytest
from fastapi import FastAPI

from app.main import app as candidate_app


@pytest.fixture
def app() -> FastAPI:
    return candidate_app


@pytest.fixture
async def client(app: FastAPI) -> AsyncIterator[httpx.AsyncClient]:
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app),
        base_url="http://test",
    ) as test_client:
        yield test_client
