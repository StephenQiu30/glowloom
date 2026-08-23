from fastapi import APIRouter

from app.modules.governance.audit.api import router as audit_router
from app.modules.governance.consents.api import router as consents_router

router = APIRouter(prefix="/api/v1", tags=["governance"])
router.include_router(audit_router)
router.include_router(consents_router)
