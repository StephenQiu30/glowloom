from fastapi import APIRouter

from app.modules.projects.episodes.api import router as episodes_router
from app.modules.projects.projects.api import router as projects_router
from app.modules.projects.snapshots.api import router as snapshots_router

router = APIRouter()
router.include_router(projects_router)
router.include_router(episodes_router)
router.include_router(snapshots_router)
