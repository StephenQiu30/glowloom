from fastapi import APIRouter

from api.routes.assets import router as asset_router
from api.routes.projects import router as project_router
from api.routes.scripts import router as script_router
from api.routes.storyboards import router as storyboard_router
from api.routes.tasks import router as task_router

router = APIRouter()
router.include_router(project_router)
router.include_router(task_router)
router.include_router(script_router)
router.include_router(asset_router)
router.include_router(storyboard_router)
