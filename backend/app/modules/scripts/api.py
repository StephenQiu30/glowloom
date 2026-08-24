from fastapi import APIRouter

from app.modules.scripts.adaptations.api import router as adaptations_router
from app.modules.scripts.documents.api import router as documents_router
from app.modules.scripts.extractions.api import router as extractions_router
from app.modules.scripts.narratives.api import router as narratives_router
from app.modules.scripts.planning.api import router as planning_router
from app.modules.scripts.production_bibles.api import router as production_bibles_router
from app.modules.scripts.structure.api import router as structure_router
from app.modules.scripts.versions.api import lookup_router as versions_lookup_router
from app.modules.scripts.versions.api import router as versions_router

router = APIRouter()
router.include_router(adaptations_router)
router.include_router(documents_router)
router.include_router(versions_router)
router.include_router(extractions_router)
router.include_router(planning_router)
router.include_router(production_bibles_router)
router.include_router(narratives_router)
router.include_router(structure_router)
router.include_router(versions_lookup_router)
