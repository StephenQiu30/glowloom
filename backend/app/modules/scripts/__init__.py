"""Public script extraction application use cases."""

from app.modules.scripts.contracts import (
    ConfirmedShotCandidateReference,
    ConfirmedStructureQuery,
    ConfirmedStructureReference,
    ScriptExtractionInput,
    ScriptProductionSummary,
)
from app.modules.scripts.extractions.ports import (
    ScriptExtractionProviderError,
    ScriptStructureExtractor,
)
from app.modules.scripts.extractions.schemas import ScriptExtractionResult
from app.modules.scripts.extractions.service import (
    get_script_extraction_input,
    record_extraction_result,
    summarize_current_scripts,
    synchronize_extraction_batch_status,
)
from app.modules.scripts.structure.service import resolve_confirmed_shot_candidate
from app.modules.scripts.versions.service import (
    resolve_confirmed_structure,
    resolve_confirmed_structures,
    script_version_exists,
)

__all__ = [
    "record_extraction_result",
    "ConfirmedStructureReference",
    "ConfirmedStructureQuery",
    "ConfirmedShotCandidateReference",
    "get_script_extraction_input",
    "ScriptExtractionInput",
    "ScriptExtractionProviderError",
    "ScriptExtractionResult",
    "ScriptProductionSummary",
    "ScriptStructureExtractor",
    "resolve_confirmed_structure",
    "resolve_confirmed_structures",
    "resolve_confirmed_shot_candidate",
    "script_version_exists",
    "summarize_current_scripts",
    "synchronize_extraction_batch_status",
]
