from __future__ import annotations

from collections.abc import Iterable
from typing import Any, cast

from pydantic import BaseModel

from app.text_contract.schemas import (
    BlockRange,
    Episode,
    EpisodeAnalysis,
    EpisodeMap,
    Evidence,
    Issue,
    Mention,
    MentionRef,
    Scene,
    SceneDirection,
    WorldBook,
)
from app.text_contract.source import (
    ResolvedEvidence,
    SourceEdition,
    inspect_source,
    resolve_evidence,
)


def unique(values: Iterable[str], label: str) -> set[str]:
    items = list(values)
    if len(items) != len(set(items)):
        raise ValueError(f"duplicate {label}")
    return set(items)


def block_set(value: BlockRange) -> set[int]:
    return set(range(value.first_block, value.last_block + 1))


def coverage(expected: set[int], ranges: Iterable[BlockRange]) -> None:
    seen: set[int] = set()
    for value in ranges:
        if value.first_block not in expected or value.last_block not in expected:
            raise ValueError("source coverage exceeds scope")
        current = block_set(value)
        if current & seen or not current <= expected:
            raise ValueError("source coverage overlaps or exceeds scope")
        seen.update(current)
    if seen != expected:
        raise ValueError("source coverage has missing blocks")


def evidence_in(value: BaseModel) -> list[Evidence]:
    found: list[Evidence] = []

    def visit(item: Any) -> None:
        if isinstance(item, Evidence):
            found.append(item)
        elif isinstance(item, BaseModel):
            for name in type(item).model_fields:
                visit(getattr(item, name))
        elif isinstance(item, list):
            for child in cast(list[Any], item):
                visit(child)

    visit(value)
    return found


def check_evidence(
    source: SourceEdition, value: BaseModel, allowed: set[int] | None = None
) -> list[ResolvedEvidence]:
    blocks = inspect_source(source)
    result: dict[tuple[int, int], ResolvedEvidence] = {}
    for evidence in evidence_in(value):
        if allowed is not None and evidence.block not in allowed:
            raise ValueError("evidence is outside the task scope")
        resolved = resolve_evidence(source, evidence, blocks)
        result[resolved.start, resolved.end] = resolved
    return list(result.values())


def check_episode_map(source: SourceEdition, draft: EpisodeMap) -> list[Issue]:
    blocks = inspect_source(source)
    unique((episode.key for episode in draft.episodes), "episode key")
    coverage(set(range(len(blocks))), [*draft.episodes, *draft.excluded])
    if [episode.first_block for episode in draft.episodes] != sorted(
        episode.first_block for episode in draft.episodes
    ):
        raise ValueError("episodes must retain source presentation order")
    issues = list(draft.issues)
    numbers: list[int] = []
    for episode in draft.episodes:
        headings = [
            block
            for block in blocks[episode.first_block : episode.last_block + 1]
            if block.heading_number is not None
        ]
        if draft.mode == "preserve":
            if len(headings) != 1 or headings[0].heading_number != episode.number:
                raise ValueError("episode heading and preserved number differ")
            if headings[0].index != episode.first_block:
                raise ValueError("preserved episode must start at its heading")
        elif episode.number is not None:
            raise ValueError("proposed episode must not invent a source number")
        if episode.number is not None:
            numbers.append(episode.number)
    if draft.mode == "propose":
        issues.append(
            Issue(
                code="proposed_episode_boundaries",
                scope="manuscript",
                severity="blocker",
                summary="无显式集边界的拆集提案需要人工确认。",
            )
        )
    if len(numbers) != len(set(numbers)):
        issues.append(
            Issue(
                code="duplicate_episode_number",
                scope="manuscript",
                severity="blocker",
                summary="原稿出现重复集号，保留原文并请求确认。",
            )
        )
    if any(right != left + 1 for left, right in zip(numbers, numbers[1:], strict=False)):
        issues.append(
            Issue(
                code="episode_number_gap",
                scope="manuscript",
                severity="warning",
                summary="原稿集号不连续；未补造缺失剧集。",
            )
        )
    for excluded in draft.excluded:
        if excluded.kind == "unresolved":
            issues.append(
                Issue(
                    code="unresolved_source",
                    scope="manuscript",
                    severity="blocker",
                    summary=excluded.reason,
                )
            )
        if any(blocks[index].heading_number is not None for index in block_set(excluded)):
            issues.append(
                Issue(
                    code="excluded_episode_heading",
                    scope="manuscript",
                    severity="blocker",
                    summary="集标题被归类为目录或非正文，请确认。",
                )
            )
    return issues


def check_episode(source: SourceEdition, episode: Episode, draft: EpisodeAnalysis) -> list[Issue]:
    if draft.episode_key != episode.key:
        raise ValueError("episode scope differs")
    unique((scene.key for scene in draft.scenes), "scene key")
    coverage(block_set(episode), [*draft.scenes, *draft.excluded])
    if [scene.first_block for scene in draft.scenes] != sorted(
        scene.first_block for scene in draft.scenes
    ):
        raise ValueError("scenes must retain presentation order")
    issues = list(draft.issues)
    for scene in draft.scenes:
        check_evidence(source, scene, block_set(scene))
        unique((beat.key for beat in scene.beats), "beat key")
        unique((dialogue.key for dialogue in scene.dialogues), "dialogue key")
        unique((mention.key for mention in scene.mentions), "mention key")
        cast_keys = {mention.key for mention in scene.mentions if mention.kind == "cast"}
        for dialogue in scene.dialogues:
            if dialogue.text != dialogue.evidence.quote:
                raise ValueError("dialogue text must equal its exact source quote")
            if dialogue.speaker_mention is not None and dialogue.speaker_mention not in cast_keys:
                raise ValueError("dialogue speaker must reference a scene cast mention")
        for mention in scene.mentions:
            if mention.name not in mention.evidence.quote:
                raise ValueError("mention name must occur in its source evidence")
        issues.extend(scene.issues)
    for item in draft.excluded:
        if item.kind == "unresolved":
            issues.append(
                Issue(
                    code="unresolved_source",
                    scope=episode.key,
                    severity="blocker",
                    summary=item.reason,
                )
            )
    return issues


def mention_key(ref: MentionRef) -> tuple[str, str, str]:
    return ref.episode_key, ref.scene_key, ref.mention_key


def check_world(
    source: SourceEdition, analyses: list[EpisodeAnalysis], world: WorldBook
) -> list[Issue]:
    entities = unique((entity.key for entity in world.entities), "entity key")
    mentions: dict[tuple[str, str, str], Mention] = {}
    scenes: dict[tuple[str, str], Scene] = {}
    for analysis in analyses:
        for scene in analysis.scenes:
            scenes[analysis.episode_key, scene.key] = scene
            for mention in scene.mentions:
                mentions[analysis.episode_key, scene.key, mention.key] = mention
    seen: set[tuple[str, str, str]] = set()
    issues = list(world.issues)
    check_evidence(source, world)
    identity_evidence = {
        (resolved.start, resolved.end)
        for entity in world.entities
        for resolved in check_evidence(source, entity)
    }
    for entity in world.entities:
        for ref in entity.mentions:
            key = mention_key(ref)
            if key not in mentions or key in seen:
                raise ValueError("entity references missing or already assigned mention")
            if mentions[key].kind != entity.kind:
                raise ValueError("entity and mention kind differ")
            seen.add(key)
        if entity.identity_basis != "explicit":
            if entity.uncertainty is None:
                raise ValueError("uncertain identity requires an explanation")
            issues.append(
                Issue(
                    code="identity_requires_review",
                    scope=entity.key,
                    severity="blocker",
                    summary=entity.uncertainty,
                )
            )
    for ref in world.unresolved_mentions:
        key = mention_key(ref)
        if key not in mentions or key in seen:
            raise ValueError("unresolved mention is missing or already assigned")
        seen.add(key)
    if seen != set(mentions):
        raise ValueError("world mention coverage is incomplete")
    if world.unresolved_mentions:
        issues.append(
            Issue(
                code="unresolved_identity",
                scope="world",
                severity="blocker",
                summary="仍有未归并的场次提及。",
            )
        )
    for relation in world.relations:
        if relation.subject not in entities or relation.target not in entities:
            raise ValueError("relation references missing entity")
    for event in world.state_events:
        scene = scenes.get((event.episode_key, event.scene_key))
        if event.entity_key not in entities or scene is None:
            raise ValueError("state event references missing entity or scene")
        if event.time_branch != scene.time_branch:
            raise ValueError("state event cannot overwrite a different time branch")
        local = block_set(scene)
        if not any(item.block in local for item in event.evidence):
            raise ValueError("state event requires evidence in its own scene")
        for evidence in event.evidence:
            if evidence.block not in local:
                resolved = resolve_evidence(source, evidence)
                if (resolved.start, resolved.end) not in identity_evidence:
                    raise ValueError("state event references unsupported cross-scene evidence")
    for need in world.asset_needs:
        if need.entity_key not in entities:
            raise ValueError("asset need references missing entity")
    return issues


def check_direction(
    source: SourceEdition, episode_key: str, scene: Scene, draft: SceneDirection
) -> list[Issue]:
    if (draft.episode_key, draft.scene_key) != (episode_key, scene.key):
        raise ValueError("direction scope differs")
    unique((shot.key for shot in draft.shots), "shot key")
    beats = {beat.key for beat in scene.beats}
    required = {beat.key for beat in scene.beats if beat.required}
    dialogues = {dialogue.key: dialogue for dialogue in scene.dialogues}
    visible = {mention.key for mention in scene.mentions if mention.presence == "onscreen"}
    detail_refs = {
        (resolved.start, resolved.end)
        for mention in scene.mentions
        if mention.presence == "onscreen"
        for evidence in mention.visual_details
        for resolved in [resolve_evidence(source, evidence)]
    }
    covered_beats: set[str] = set()
    covered_dialogues: set[str] = set()
    covered_details: set[tuple[int, int]] = set()
    for blocking in draft.blocking:
        if blocking.mention_key not in visible:
            raise ValueError("blocking requires an onscreen presence")
    check_evidence(source, draft, block_set(scene))
    for shot in draft.shots:
        if not set(shot.beat_keys) <= beats or not set(shot.visible_mentions) <= visible:
            raise ValueError("shot references missing beat or non-visible presence")
        covered_beats.update(shot.beat_keys)
        covered_details.update(
            (resolved.start, resolved.end)
            for item in shot.detail_evidence
            for resolved in [resolve_evidence(source, item)]
        )
        for cue in shot.audio:
            if cue.dialogue_key not in dialogues:
                raise ValueError("shot references missing dialogue")
            if cue.channel != dialogues[cue.dialogue_key].channel:
                raise ValueError("shot cannot silently change dialogue routing")
            covered_dialogues.add(cue.dialogue_key)
    if not required <= covered_beats or set(dialogues) != covered_dialogues:
        raise ValueError("shot coverage omits required beats or dialogue")
    if not detail_refs <= covered_details:
        raise ValueError("shot coverage omits required visible source details")
    return list(draft.issues)
