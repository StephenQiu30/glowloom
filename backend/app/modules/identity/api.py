from fastapi import APIRouter

from app.modules.identity.authentication.api import router as authentication_router
from app.modules.identity.workspaces.api import router as workspaces_router

router = APIRouter()
router.include_router(authentication_router)
router.include_router(workspaces_router)
