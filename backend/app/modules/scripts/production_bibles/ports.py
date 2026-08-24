from dataclasses import dataclass
from typing import Literal, Protocol
from uuid import UUID

from app.modules.scripts.production_bibles.schemas import ProductionBibleProviderResult

PRODUCTION_BIBLE_ENGINE_VERSION = "production-bible-agent-v1"
PRODUCTION_BIBLE_HARNESS_VERSION = "production-bible-harness-v1"
PRODUCTION_BIBLE_PROMPT_VERSION = "production-bible-prompt-v1"
PRODUCTION_BIBLE_SCHEMA_VERSION = "production-bible-schema-v1"


@dataclass(frozen=True, slots=True)
class ProductionBibleInput:
    bible_id: UUID
    task_id: UUID
    workspace_id: UUID
    project_id: UUID
    document_revision_id: UUID
    input_hash: str
    normalized_text: str
    run_token: UUID


class ProductionBibleBuilder(Protocol):
    async def build(
        self,
        bible_input: ProductionBibleInput,
    ) -> ProductionBibleProviderResult: ...


class ProductionBibleProviderError(Exception):
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


class ProductionBibleLeaseLost(RuntimeError):
    pass
