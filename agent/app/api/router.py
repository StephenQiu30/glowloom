"""Compose the internal capability routers owned by the Agent service."""

from fastapi import APIRouter

from app.candidate_runtime.api import router as candidate_router


def build_router() -> APIRouter:
    """Return the internal router without changing any capability route contract."""

    router = APIRouter()
    router.include_router(candidate_router)
    return router
