from fastapi import APIRouter

from app.modules.governance.consents.api import router as consents_router

router = APIRouter(prefix="/api/v1", tags=["governance"])
router.include_router(consents_router)
