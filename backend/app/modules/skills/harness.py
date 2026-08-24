from collections.abc import Iterable
from typing import Any, TypeVar, cast

from langchain_core.messages import HumanMessage, SystemMessage
from langgraph.graph import END, START, StateGraph  # pyright: ignore[reportMissingTypeStubs]
from pydantic import BaseModel, ValidationError
from typing_extensions import TypedDict

from app.modules.skills.contracts import (
    SkillDefinition,
    SkillExecutionContext,
    SkillExecutionError,
    SkillRun,
    StructuredSkillModel,
    input_hash,
)

T = TypeVar("T", bound=BaseModel)


class _SkillGraphState(TypedDict, total=False):
    system_prompt: str
    user_payload: str
    raw_output: object
    output: BaseModel


SKILL_WORKFLOW_VERSION = "langgraph-state-graph-v1"


class SkillHarness:
    """Runs a typed Skill without granting it domain write access."""

    def __init__(self, *, checkpointer: Any | None = None) -> None:
        # MVP keeps Task/Outbox/Inbox as the durable product fact. A persistent
        # LangGraph checkpointer can be injected later for multi-step HITL runs.
        self._checkpointer = checkpointer

    async def run(
        self,
        *,
        skill: SkillDefinition,
        model: StructuredSkillModel,
        system_prompt: str,
        user_payload: str,
        output_model: type[T],
        context: SkillExecutionContext | None = None,
    ) -> SkillRun[T]:
        execution_context = context or SkillExecutionContext(
            skill_name=skill.name,
            skill_version=skill.version,
        )
        if (
            execution_context.skill_name != skill.name
            or execution_context.skill_version != skill.version
        ):
            raise SkillExecutionError(
                outcome="failed",
                code="skill_context_invalid",
                summary="Skill execution context does not match the Skill",
                retryable=False,
                next_action="contact_support",
            )
        graph = self._build_graph(
            skill=skill,
            model=model,
            output_model=output_model,
        )
        graph_config = {
            "configurable": {
                "thread_id": execution_context.task_id or input_hash(user_payload),
            }
        }
        result = await graph.ainvoke(
            {
                "system_prompt": system_prompt,
                "user_payload": user_payload,
            },
            config=graph_config,
        )
        output = cast(T, result["output"])

        return SkillRun(
            output=output,
            skill_name=skill.name,
            skill_version=skill.version,
            input_hash=input_hash(user_payload),
            trace_id=execution_context.trace_id,
        )

    def _build_graph(
        self,
        *,
        skill: SkillDefinition,
        model: StructuredSkillModel,
        output_model: type[T],
    ) -> Any:
        async def validate_input(state: _SkillGraphState) -> dict[str, object]:
            system_prompt = state.get("system_prompt", "")
            user_payload = state.get("user_payload", "")
            if not system_prompt.strip() or not user_payload.strip():
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_input_invalid",
                    summary="Skill input is empty",
                    retryable=False,
                    next_action="fix_skill_input",
                )
            if len(user_payload) > skill.max_input_chars:
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_input_too_large",
                    summary="Skill input exceeds the Skill limit",
                    retryable=False,
                    next_action="reduce_skill_input",
                )
            return {}

        async def invoke_model(state: _SkillGraphState) -> dict[str, object]:
            raw_output = await model.ainvoke(
                [
                    SystemMessage(content=state.get("system_prompt", "")),
                    HumanMessage(content=state.get("user_payload", "")),
                ]
            )
            return {"raw_output": raw_output}

        async def validate_output(state: _SkillGraphState) -> dict[str, object]:
            try:
                output = output_model.model_validate(state.get("raw_output"))
            except (TypeError, ValueError, ValidationError) as error:
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_output_invalid",
                    summary="Skill returned an invalid structured result",
                    retryable=False,
                    next_action="start_new_skill_run",
                ) from error
            return {"output": output}

        async def candidate_gate(state: _SkillGraphState) -> dict[str, object]:
            del state
            if not skill.candidate_only:
                raise SkillExecutionError(
                    outcome="failed",
                    code="skill_side_effect_policy_invalid",
                    summary="MVP Skills must produce candidates only",
                    retryable=False,
                    next_action="contact_support",
                )
            return {}

        # LangGraph's current stubs cannot infer the TypedDict schema from the
        # runtime constructor. Keep the graph builder behind the boundary while
        # retaining the typed state and node signatures above.
        builder: Any = StateGraph(_SkillGraphState)
        builder.add_node("validate_input", validate_input)
        builder.add_node("invoke_model", invoke_model)
        builder.add_node("validate_output", validate_output)
        builder.add_node("candidate_gate", candidate_gate)
        builder.add_edge(START, "validate_input")
        builder.add_edge("validate_input", "invoke_model")
        builder.add_edge("invoke_model", "validate_output")
        builder.add_edge("validate_output", "candidate_gate")
        builder.add_edge("candidate_gate", END)
        if self._checkpointer is None:
            return builder.compile()
        return builder.compile(checkpointer=self._checkpointer)

    def require_tools(
        self,
        skill: SkillDefinition,
        requested_tools: Iterable[str],
    ) -> None:
        denied = set(requested_tools).difference(skill.allowed_tools)
        if denied:
            raise SkillExecutionError(
                outcome="failed",
                code="skill_tool_denied",
                summary="Skill requested a tool outside its allowlist",
                retryable=False,
                next_action="contact_support",
            )
