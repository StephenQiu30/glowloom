"""Public script extraction application use cases."""

from app.modules.scripts.adaptations.ports import ScriptAdaptationProviderError
from app.modules.scripts.adaptations.schemas import ScriptAdaptationProviderResult
from app.modules.scripts.adaptations.service import (
    AdaptationInput,
    AdaptationInputChanged,
    prepare_adaptation_input,
    record_adaptation_error,
    record_adaptation_result,
)
from app.modules.scripts.contracts import (
    ConfirmedShotCandidateReference,
    ConfirmedStructureQuery,
    ConfirmedStructureReference,
    EpisodeConfirmedStructureQuery,
    NarrativeDependencySnapshot,
    ProductionBibleExtractionInput,
    ScriptExtractionInput,
    ScriptProductionSummary,
    ScriptVersionSnapshot,
    StoryboardNarrativeSnapshot,
)
from app.modules.scripts.documents import analyze_document
from app.modules.scripts.extractions.ports import (
    ScriptExtractionProviderError,
    ScriptStructureExtractor,
)
from app.modules.scripts.extractions.schemas import (
    AssetOccurrenceCandidateProposal,
    ContinuityCandidateProposal,
    DialogueCandidateProposal,
    ScriptExtractionResult,
    ShotCandidateProposal,
)
from app.modules.scripts.extractions.service import (
    count_asset_candidate_decisions,
    get_script_extraction_input,
    record_extraction_result,
    resolve_confirmed_script_production_bible_context,
    summarize_current_scripts,
    synchronize_extraction_batch_status,
)
from app.modules.scripts.narratives.service import (
    resolve_narrative_dependencies,
    resolve_narrative_unit_versions,
    resolve_storyboard_narrative,
)
from app.modules.scripts.planning.ports import (
    EpisodePlanner,
    EpisodePlanningInput,
    EpisodePlanningProviderError,
)
from app.modules.scripts.planning.schemas import EpisodePlanningProviderResult
from app.modules.scripts.planning.service import (
    get_episode_planning_input,
    record_episode_planning_error,
    record_episode_planning_result,
)
from app.modules.scripts.production_bibles import (
    ProductionBibleInput,
    ProductionBibleInputChanged,
    ProductionBibleLeaseActive,
    ProductionBibleLeaseLost,
    ProductionBibleProviderError,
    ProductionBibleProviderResult,
    ProductionBibleResumeRequest,
    fence_bible_run,
    prepare_bible_input,
    record_bible_error,
    record_bible_result,
    resume_bible,
)
from app.modules.scripts.structure.service import resolve_confirmed_shot_candidate
from app.modules.scripts.versions.service import (
    count_episode_script_versions,
    resolve_confirmed_structure,
    resolve_confirmed_structures,
    resolve_episode_confirmed_structures,
    resolve_script_version_snapshot,
    script_version_exists,
)

__all__ = [
    "AdaptationInput",
    "AdaptationInputChanged",
    "analyze_document",
    "count_asset_candidate_decisions",
    "record_extraction_result",
    "resolve_confirmed_script_production_bible_context",
    "ConfirmedStructureReference",
    "ConfirmedStructureQuery",
    "EpisodeConfirmedStructureQuery",
    "NarrativeDependencySnapshot",
    "StoryboardNarrativeSnapshot",
    "ConfirmedShotCandidateReference",
    "ContinuityCandidateProposal",
    "AssetOccurrenceCandidateProposal",
    "DialogueCandidateProposal",
    "count_episode_script_versions",
    "EpisodePlanner",
    "EpisodePlanningInput",
    "EpisodePlanningProviderError",
    "EpisodePlanningProviderResult",
    "ProductionBibleInput",
    "ProductionBibleExtractionInput",
    "ProductionBibleInputChanged",
    "ProductionBibleLeaseActive",
    "ProductionBibleLeaseLost",
    "ProductionBibleProviderError",
    "ProductionBibleProviderResult",
    "ProductionBibleResumeRequest",
    "get_episode_planning_input",
    "get_script_extraction_input",
    "fence_bible_run",
    "prepare_bible_input",
    "ScriptExtractionInput",
    "ScriptExtractionProviderError",
    "ScriptExtractionResult",
    "ShotCandidateProposal",
    "ScriptAdaptationProviderError",
    "ScriptAdaptationProviderResult",
    "ScriptProductionSummary",
    "ScriptVersionSnapshot",
    "ScriptStructureExtractor",
    "record_episode_planning_error",
    "record_episode_planning_result",
    "record_bible_error",
    "record_bible_result",
    "resume_bible",
    "prepare_adaptation_input",
    "record_adaptation_error",
    "record_adaptation_result",
    "resolve_confirmed_structure",
    "resolve_confirmed_structures",
    "resolve_episode_confirmed_structures",
    "resolve_script_version_snapshot",
    "resolve_narrative_dependencies",
    "resolve_narrative_unit_versions",
    "resolve_storyboard_narrative",
    "resolve_confirmed_shot_candidate",
    "script_version_exists",
    "summarize_current_scripts",
    "synchronize_extraction_batch_status",
]
