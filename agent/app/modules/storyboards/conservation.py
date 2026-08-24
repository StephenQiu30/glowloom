from collections.abc import Sequence
from typing import Literal

from app.modules.storyboards.schemas import (
    DialogueOrNarration,
    NarrativeReferenceInput,
    ShotSpec,
    TargetShotSpecRequest,
)

ConservationCode = Literal[
    "split_source_mismatch",
    "split_duration_mismatch",
    "split_action_mismatch",
    "split_dialogue_id_mismatch",
    "split_dialogue_mismatch",
    "merge_script_mismatch",
    "merge_scene_mismatch",
    "merge_duration_overflow",
    "merge_action_overflow",
    "merge_dialogue_overflow",
    "merge_dialogue_duplicate",
    "merge_target_mismatch",
    "merge_duration_mismatch",
    "merge_action_mismatch",
    "merge_dialogue_id_mismatch",
    "merge_dialogue_mismatch",
    "split_narrative_mismatch",
    "merge_narrative_conflict",
    "merge_narrative_mismatch",
]


class TransformConservationError(ValueError):
    def __init__(self, code: ConservationCode, summary: str) -> None:
        super().__init__(summary)
        self.code = code
        self.summary = summary

    def __str__(self) -> str:
        return self.summary


def _raise(code: ConservationCode, summary: str) -> None:
    raise TransformConservationError(code=code, summary=summary)


def _action_signature(spec: ShotSpec) -> list[tuple[str, str]]:
    return [(beat.beat_key, beat.description) for beat in spec.action_beats]


def _split_dialogue_signature(item: DialogueOrNarration) -> tuple[object, ...]:
    return (
        item.source_dialogue_id,
        item.beat_key,
        item.speaker_subject_key,
        item.render_as_audio,
        item.performance_note,
    )


def validate_split_content(
    source: ShotSpec,
    targets: Sequence[TargetShotSpecRequest],
) -> None:
    if len(targets) != 2:
        _raise("split_source_mismatch", "split requires exactly two targets")
    target_specs = [target.spec for target in targets]
    source_reference = source.script_reference
    if any(
        target.script_reference.confirmed_script_version_id
        != source_reference.confirmed_script_version_id
        or target.script_reference.scene_id != source_reference.scene_id
        for target in target_specs
    ):
        _raise(
            "split_source_mismatch",
            "split targets must preserve the source script version and scene",
        )
    if sum(target.duration_ms for target in target_specs) != source.duration_ms:
        _raise(
            "split_duration_mismatch",
            "split target durations must equal the source duration",
        )

    target_actions = [
        signature for target in target_specs for signature in _action_signature(target)
    ]
    if target_actions != _action_signature(source):
        _raise(
            "split_action_mismatch",
            "split action beats must form one ordered exact source partition",
        )

    target_dialogue_ids = [
        dialogue_id
        for target in target_specs
        for dialogue_id in target.script_reference.dialogue_ids
    ]
    if target_dialogue_ids != source_reference.dialogue_ids:
        _raise(
            "split_dialogue_id_mismatch",
            "split dialogue IDs must form one ordered exact source partition",
        )

    target_dialogues = [
        _split_dialogue_signature(item)
        for target in target_specs
        for item in target.dialogue_or_narration
    ]
    source_dialogues = [_split_dialogue_signature(item) for item in source.dialogue_or_narration]
    if target_dialogues != source_dialogues:
        _raise(
            "split_dialogue_mismatch",
            "split dialogue content must form one ordered exact source partition",
        )


def _merge_dialogue_signature(
    item: DialogueOrNarration,
    *,
    mapped_beat_key: str | None,
) -> tuple[object, ...]:
    return (
        item.source_dialogue_id,
        mapped_beat_key,
        item.speaker_subject_key,
        item.render_as_audio,
        item.performance_note,
    )


def validate_merge_content(
    sources: Sequence[ShotSpec],
    target: TargetShotSpecRequest | None,
) -> None:
    if len(sources) != 2:
        _raise("merge_script_mismatch", "merge requires exactly two source specs")
    script_ids = {source.script_reference.confirmed_script_version_id for source in sources}
    if len(script_ids) != 1:
        _raise(
            "merge_script_mismatch",
            "merge sources must use the same confirmed script version",
        )
    scene_ids = {source.script_reference.scene_id for source in sources}
    if len(scene_ids) != 1:
        _raise("merge_scene_mismatch", "merge sources must use the same scene")

    duration_ms = sum(source.duration_ms for source in sources)
    if duration_ms > 15_000:
        _raise(
            "merge_duration_overflow",
            "merge source duration cannot exceed 15 seconds",
        )
    source_actions = [beat for source in sources for beat in source.action_beats]
    if len(source_actions) > 8:
        _raise(
            "merge_action_overflow",
            "merge can represent at most 8 action beats",
        )
    source_dialogue_ids = [
        dialogue_id for source in sources for dialogue_id in source.script_reference.dialogue_ids
    ]
    source_dialogues = [item for source in sources for item in source.dialogue_or_narration]
    if len(source_dialogue_ids) > 8 or len(source_dialogues) > 8:
        _raise(
            "merge_dialogue_overflow",
            "merge can represent at most 8 dialogues",
        )
    if len(set(source_dialogue_ids)) != len(source_dialogue_ids):
        _raise(
            "merge_dialogue_duplicate",
            "merge sources contain a duplicate dialogue identity",
        )
    if target is None:
        return

    target_spec = target.spec
    if (
        target_spec.script_reference.confirmed_script_version_id not in script_ids
        or target_spec.script_reference.scene_id not in scene_ids
    ):
        _raise(
            "merge_target_mismatch",
            "merge target must preserve the source script version and scene",
        )
    if target_spec.duration_ms != duration_ms:
        _raise(
            "merge_duration_mismatch",
            "merge target duration must equal the source durations",
        )
    if [beat.description for beat in target_spec.action_beats] != [
        beat.description for beat in source_actions
    ]:
        _raise(
            "merge_action_mismatch",
            "merge action beats must preserve every source action in order",
        )
    if target_spec.script_reference.dialogue_ids != source_dialogue_ids:
        _raise(
            "merge_dialogue_id_mismatch",
            "merge dialogue IDs must preserve every source dialogue in order",
        )

    target_beat_keys = [beat.beat_key for beat in target_spec.action_beats]
    expected_dialogues: list[tuple[object, ...]] = []
    action_offset = 0
    for source in sources:
        source_action_indexes = {
            beat.beat_key: action_offset + index for index, beat in enumerate(source.action_beats)
        }
        for item in source.dialogue_or_narration:
            mapped_beat_key = (
                target_beat_keys[source_action_indexes[item.beat_key]]
                if item.beat_key is not None
                else None
            )
            expected_dialogues.append(
                _merge_dialogue_signature(item, mapped_beat_key=mapped_beat_key)
            )
        action_offset += len(source.action_beats)
    target_dialogues = [
        _merge_dialogue_signature(item, mapped_beat_key=item.beat_key)
        for item in target_spec.dialogue_or_narration
    ]
    if target_dialogues != expected_dialogues:
        _raise(
            "merge_dialogue_mismatch",
            "merge dialogue content and action links must preserve every source item",
        )


def _narrative_signature(value: NarrativeReferenceInput) -> tuple[object, ...]:
    return (
        value.unit_version_id,
        value.channel,
        value.role,
        value.coverage_mode,
        value.segment_start,
        value.segment_end,
        value.contribution,
    )


def _narrative_edge_key(value: NarrativeReferenceInput) -> tuple[object, ...]:
    return (
        value.unit_version_id,
        value.channel,
        value.segment_start,
        value.segment_end,
    )


def validate_split_narrative(
    source: Sequence[NarrativeReferenceInput],
    targets: Sequence[TargetShotSpecRequest],
) -> None:
    source_signatures = {_narrative_signature(value) for value in source}
    target_signatures = [
        _narrative_signature(value) for target in targets for value in target.narrative_references
    ]
    if (
        len(set(target_signatures)) != len(target_signatures)
        or set(target_signatures) != source_signatures
    ):
        _raise(
            "split_narrative_mismatch",
            "split narrative references must form one exact source partition",
        )


def validate_merge_narrative(
    sources: Sequence[Sequence[NarrativeReferenceInput]],
    target: TargetShotSpecRequest,
) -> None:
    source_signatures = [_narrative_signature(value) for source in sources for value in source]
    source_edges = [_narrative_edge_key(value) for source in sources for value in source]
    if len(set(source_edges)) != len(source_edges):
        _raise(
            "merge_narrative_conflict",
            "merge sources contain conflicting narrative references",
        )
    target_signatures = {_narrative_signature(value) for value in target.narrative_references}
    if target_signatures != set(source_signatures):
        _raise(
            "merge_narrative_mismatch",
            "merge target must preserve every source narrative reference",
        )
