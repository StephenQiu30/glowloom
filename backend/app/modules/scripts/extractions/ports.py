from typing import Literal, Protocol

from app.modules.scripts.extractions.schemas import ScriptExtractionResult

SCRIPT_STRUCTURE_EXTRACTOR_VERSION = (
    "deepseek-v4-pro:thinking-off:lc-deepseek-1.1.0:prompt-v1:schema-v1"
)


class ScriptStructureExtractor(Protocol):
    async def extract(self, script_body: str) -> ScriptExtractionResult: ...


class ScriptExtractionProviderError(Exception):
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
