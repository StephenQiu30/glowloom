from __future__ import annotations

import json
import os
import shutil
from collections.abc import Awaitable, Callable
from pathlib import Path
from typing import TYPE_CHECKING, Any, cast

from pydantic import BaseModel

from app.protocol.canonical import canonical_hash, canonical_json
from app.reasoning.codex import (
    codex_output_schema,
    run_codex_process,
)
from app.text_contract.schemas import (
    EpisodeAnalysis,
    EpisodeMap,
    Issue,
    Scene,
    SceneDirection,
    WorldBook,
)
from app.text_contract.source import (
    inspect_source,
)
from app.text_contract.task import ContextManifest, Stage, TextResult, TextTask
from app.text_contract.validation import (
    block_set,
    check_direction,
    check_episode,
    check_episode_map,
    check_evidence,
    check_world,
    unique,
)

if TYPE_CHECKING:
    from app.skills.catalog import SkillCatalog


MODELS: dict[Stage, type[BaseModel]] = {
    "map_manuscript": EpisodeMap,
    "analyze_episode": EpisodeAnalysis,
    "build_world": WorldBook,
    "direct_scene": SceneDirection,
}
REFERENCES: dict[Stage, str] = {
    "map_manuscript": "references/manuscript.md",
    "analyze_episode": "references/episode.md",
    "build_world": "references/world.md",
    "direct_scene": "references/directing.md",
}
# This is an independently frozen release; it never changes the legacy StoryGraph bundle.
RELEASE_HASH = "63310724e287f09d5577bbe44410a480aefe7ad8f72956b2d78615c3ac05ecb3"


class ContextInsufficient(ValueError):
    pass


class SkillReleaseInvalid(ValueError):
    pass


class InputContractInvalid(ValueError):
    pass


class CandidateContractInvalid(ValueError):
    def __init__(self, candidate: dict[str, Any], diagnostic: str) -> None:
        super().__init__(diagnostic[:500])
        self.candidate = candidate
        self.diagnostic = diagnostic[:500]


class TextSkill:
    def __init__(self, root: Path | None = None) -> None:
        self.root = root or Path(__file__).resolve().parents[3] / "skills" / "text-storyboard"

    def release_hash(self) -> str:
        files: dict[str, str] = {}
        for name in ["SKILL.md", *REFERENCES.values()]:
            path = self.root / name
            if (
                path.is_symlink()
                or not path.is_file()
                or path.resolve().parent
                not in {self.root.resolve(), (self.root / "references").resolve()}
            ):
                raise SkillReleaseInvalid("Skill release file is missing or invalid")
            files[name] = path.read_text(encoding="utf-8")
        return canonical_hash(
            {
                "files": files,
                "schemas": {key: codex_output_schema(value) for key, value in MODELS.items()},
                "input_schema": TextTask.model_json_schema(),
                "tools": [],
                "model_calls": 1,
                "max_context_bytes": 240000,
                "max_output_bytes": 2000000,
            }
        )

    def guidance(self, stage: Stage, requested_hash: str) -> str:
        if requested_hash != RELEASE_HASH or self.release_hash() != RELEASE_HASH:
            raise SkillReleaseInvalid("requested Skill release is not installed or was changed")
        return (
            (self.root / "SKILL.md").read_text()
            + "\n\n"
            + (self.root / REFERENCES[stage]).read_text()
        )


def directing_context(world: WorldBook, episode_key: str, scene: Scene) -> dict[str, Any]:
    """Local names prevent a global alias merge from disclosing a later identity."""
    local = {mention.key: mention for mention in scene.mentions}
    entities: list[dict[str, Any]] = []
    entity_keys: set[str] = set()
    for entity in world.entities:
        refs = [
            ref
            for ref in entity.mentions
            if (ref.episode_key, ref.scene_key) == (episode_key, scene.key)
        ]
        if not refs:
            continue
        entity_keys.add(entity.key)
        # Even candidate keys may encode a hidden name; do not expose global identifiers.
        entities.append(
            {
                "kind": entity.kind,
                "local_mentions": [local[ref.mention_key].model_dump(mode="json") for ref in refs],
            }
        )
    events: list[dict[str, Any]] = []
    for event in world.state_events:
        if (
            event.entity_key in entity_keys
            and event.time_branch == scene.time_branch
            and all(evidence.block <= scene.last_block for evidence in event.evidence)
        ):
            # Free-text state properties can also encode global identity. Only expose events
            # sourced in this scene; earlier continuity requires a reviewed disclosure mapping.
            if (event.episode_key, event.scene_key) == (episode_key, scene.key):
                events.append(
                    {
                        "basis": event.basis,
                        "knowledge": event.knowledge,
                        "evidence": [item.model_dump(mode="json") for item in event.evidence],
                    }
                )
    return {
        "entities": entities,
        "local_state_evidence": events,
        "continuity_status": "requires_reviewed_disclosure_mapping",
    }


Reasoner = Callable[[str, str, type[BaseModel], int], Awaitable[BaseModel]]


async def codex_reasoner(
    guidance: str, prompt: str, output_model: type[BaseModel], timeout_seconds: int
) -> BaseModel:
    return await run_codex_process(
        codex_bin=os.getenv("CODEX_BIN", "").strip() or shutil.which("codex") or "codex",
        guidance=guidance,
        prompt=prompt,
        output_model=output_model,
        timeout_seconds=timeout_seconds,
        max_output_bytes=2000000,
        strict_output_schema=True,
    )


class TextHarness:
    def __init__(
        self,
        reasoner: Reasoner = codex_reasoner,
        skill: TextSkill | None = None,
        repository_root: Path | None = None,
        skill_catalog: SkillCatalog | None = None,
    ) -> None:
        self.reasoner = reasoner
        if skill is not None:
            self.skill = skill
        else:
            from app.skills.catalog import SkillCatalog

            catalog = skill_catalog or SkillCatalog(repository_root)
            self.skill = cast(TextSkill, catalog.load("text_storyboard"))

    def prepare(self, task: TextTask) -> tuple[str, str, ContextManifest]:
        guidance = self.skill.guidance(task.stage, task.release_hash)
        blocks = inspect_source(task.source)
        context: dict[str, Any] = {"stage": task.stage}
        selected = set(range(len(blocks)))
        upstream: list[BaseModel] = []
        omitted: list[str] = []
        if task.stage == "map_manuscript":
            if (
                task.episode_map
                or task.analyses
                or task.world
                or task.episode_key
                or task.scene_key
            ):
                raise ValueError("map task must contain only the frozen source")
        else:
            if task.episode_map is None:
                raise ValueError("task requires an episode map")
            check_episode_map(task.source, task.episode_map)
            upstream.append(task.episode_map)
            episodes = {episode.key: episode for episode in task.episode_map.episodes}
            unique((analysis.episode_key for analysis in task.analyses), "analysis episode")
            for analysis in task.analyses:
                if analysis.episode_key not in episodes:
                    raise ValueError("analysis references an unknown episode")
                check_episode(task.source, episodes[analysis.episode_key], analysis)
            upstream.extend(task.analyses)
            if task.stage == "analyze_episode":
                if (
                    task.episode_key not in episodes
                    or task.analyses
                    or task.world
                    or task.scene_key
                ):
                    raise ValueError("episode task requires only its selected episode")
                episode = episodes[task.episode_key]
                selected = block_set(episode)
                context["episode"] = episode.model_dump(mode="json")
            else:
                if {analysis.episode_key for analysis in task.analyses} != set(episodes):
                    raise ValueError("full manuscript analysis barrier is incomplete")
                if task.stage == "build_world":
                    if task.world or task.episode_key or task.scene_key:
                        raise ValueError("world task cannot include direction or prior world")
                    context["analyses"] = [
                        analysis.model_dump(mode="json") for analysis in task.analyses
                    ]
                else:
                    if task.world is None:
                        raise ValueError("direction requires a world draft")
                    check_world(task.source, task.analyses, task.world)
                    upstream.append(task.world)
                    scene = self.scene(task)
                    selected = block_set(scene)
                    context.update(
                        episode_key=task.episode_key,
                        scene=scene.model_dump(mode="json"),
                        world=directing_context(task.world, task.episode_key or "", scene),
                    )
                    omitted.extend(
                        [
                            "global_alias_labels_and_keys",
                            "future_and_other_branch_states",
                            "unreviewed_cross_scene_continuity",
                        ]
                    )
        context["source"] = {
            "revision_id": task.source.revision_id,
            "content_hash": task.source.content_hash,
            "blocks": [[block.index, block.text] for block in blocks if block.index in selected],
        }
        prompt = json.dumps(context, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
        size = len(prompt.encode()) + len(guidance.encode())
        if size > 240000:
            raise ContextInsufficient("context_insufficient: source was not truncated")
        manifest = ContextManifest(
            source_revision_id=task.source.revision_id,
            source_hash=task.source.content_hash,
            block_indices=sorted(selected),
            upstream_hashes=[canonical_hash(item.model_dump(mode="json")) for item in upstream],
            prompt_hash=canonical_hash(context),
            prompt_bytes=size,
            omitted=omitted,
        )
        return guidance, prompt, manifest

    @staticmethod
    def scene(task: TextTask) -> Scene:
        for analysis in task.analyses:
            if analysis.episode_key == task.episode_key:
                for scene in analysis.scenes:
                    if scene.key == task.scene_key:
                        return scene
        raise ValueError("selected scene is missing")

    async def execute(self, task: TextTask) -> TextResult:
        # Pydantic's frozen records still contain mutable lists. Own a validated snapshot
        # before awaiting inference so callers cannot change the input/result binding.
        task = TextTask.model_validate_json(task.model_dump_json())
        try:
            guidance, prompt, manifest = self.prepare(task)
        except (SkillReleaseInvalid, ContextInsufficient):
            raise
        except ValueError:
            raise InputContractInvalid("input_contract_invalid") from None
        model = MODELS[task.stage]
        candidate = await self.reasoner(guidance, prompt, model, task.timeout_seconds)
        if type(candidate) is not model:
            raise CandidateContractInvalid(
                candidate.model_dump(mode="json"), "reasoner returned a different task schema"
            )
        payload = candidate.model_dump(mode="json")
        if len(canonical_json(payload)) > 2000000:
            raise CandidateContractInvalid({}, "candidate exceeds output budget")
        try:
            return self.validate_candidate(task, candidate, payload, manifest)
        except ValueError as error:
            raise CandidateContractInvalid(payload, str(error)) from None

    def validate_candidate(
        self,
        task: TextTask,
        candidate: BaseModel,
        payload: dict[str, Any],
        manifest: ContextManifest,
    ) -> TextResult:
        issues: list[Issue]
        if isinstance(candidate, EpisodeMap):
            issues = check_episode_map(task.source, candidate)
        elif isinstance(candidate, EpisodeAnalysis) and task.episode_map is not None:
            episode = next(
                item for item in task.episode_map.episodes if item.key == task.episode_key
            )
            issues = check_episode(task.source, episode, candidate)
        elif isinstance(candidate, WorldBook):
            issues = check_world(task.source, task.analyses, candidate)
        elif isinstance(candidate, SceneDirection):
            issues = check_direction(
                task.source, task.episode_key or "", self.scene(task), candidate
            )
            issues.append(
                Issue(
                    code="continuity_mapping_pending",
                    scope=task.scene_key or "scene",
                    severity="blocker",
                    summary="跨场状态与身份披露仍需平台已审阅映射；本结果仅为导演草案。",
                )
            )
        else:
            raise ValueError("task has no valid result checker")
        return TextResult(
            invocation_id=task.invocation_id,
            stage=task.stage,
            input_hash=canonical_hash(task.model_dump(mode="json")),
            release_hash=task.release_hash,
            candidate_hash=canonical_hash(payload),
            candidate=payload,
            evidence=check_evidence(task.source, candidate),
            issues=issues,
            context=manifest,
        )
