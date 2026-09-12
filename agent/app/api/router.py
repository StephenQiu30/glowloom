"""Root router composition for the Agent HTTP API."""

from fastapi import APIRouter

from app.api.routes.creation import router as creation_router
from app.api.routes.health import router as health_router
from app.api.routes.reference_plan import router as reference_plan_router
from app.api.routes.scene_analysis import router as scene_analysis_router
from app.api.routes.storygraph import router as storygraph_router
from app.api.routes.text_storyboard import router as text_storyboard_router
from app.api.routes.visual_foundation import router as visual_foundation_router

router = APIRouter()
router.include_router(health_router)
router.include_router(creation_router)
router.include_router(storygraph_router)
router.include_router(scene_analysis_router)
router.include_router(reference_plan_router)
router.include_router(visual_foundation_router)
router.include_router(text_storyboard_router)


# Kept as a small function for callers that prefer an explicit router factory.
def build_router() -> APIRouter:
    return router
