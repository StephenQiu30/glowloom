from typing import Literal, Protocol

from app.modules.scripts.adaptations.schemas import ScriptAdaptationProviderResult

SCRIPT_ADAPTATION_ENGINE_VERSION = "script-adaptation-v1"
SCRIPT_ADAPTATION_PROMPT_VERSION = "script-adaptation-prompt-v1"
SCRIPT_ADAPTATION_SCHEMA_VERSION = "script-adaptation-schema-v1"
SCRIPT_ADAPTATION_DURATION_TOLERANCE = 0.25


def adaptation_duration_bounds(target_duration_ms: int) -> tuple[int, int]:
    return (
        round(target_duration_ms * (1 - SCRIPT_ADAPTATION_DURATION_TOLERANCE)),
        round(target_duration_ms * (1 + SCRIPT_ADAPTATION_DURATION_TOLERANCE)),
    )


class ScriptAdaptationProviderError(RuntimeError):
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


class ScriptAdapter(Protocol):
    async def adapt(
        self,
        script_body: str,
        *,
        target_duration_ms: int,
        core_plot_points: list[str],
        pacing: str,
        colloquial_dialogue: bool,
    ) -> ScriptAdaptationProviderResult | dict[str, object]: ...
