from typing import Literal, Protocol

from app.modules.scripts.contracts import ProductionBibleExtractionInput
from app.modules.scripts.extractions.schemas import ScriptExtractionResult

SCRIPT_STRUCTURE_EXTRACTOR_VERSION = "langgraph-map-reduce-v1:prompt-v6:schema-v4:anchor-v2"


class ScriptStructureExtractor(Protocol):
    async def extract(
        self,
        script_body: str,
        *,
        trace_id: str | None = None,
        episode_number: int | None = None,
        production_bible: ProductionBibleExtractionInput | None = None,
    ) -> ScriptExtractionResult: ...


class ScriptExtractionProviderError(Exception):
    outcome: Literal["failed", "unknown"]

    def __init__(
        self,
        *,
        outcome: Literal["failed", "unknown"],
        code: str,
        summary: str,
        retryable: bool,
        next_action: str,
    ) -> None:
        super().__init__(summary)
        self.outcome = outcome
        self.code = code
        self.summary = summary
        self.retryable = retryable
        self.next_action = next_action
