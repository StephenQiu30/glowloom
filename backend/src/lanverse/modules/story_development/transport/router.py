from fastapi import APIRouter

from lanverse.modules.story_development.transport.asset_router import router as asset_router
from lanverse.modules.story_development.transport.script_router import router as script_router
from lanverse.modules.story_development.transport.storyboard_router import (
    router as storyboard_router,
)

router = APIRouter()
router.include_router(script_router)
router.include_router(asset_router)
router.include_router(storyboard_router)
