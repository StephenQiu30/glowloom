from __future__ import annotations

from typing import Annotated, Literal, Self

from pydantic import BaseModel, ConfigDict, Field, model_validator

Key = Annotated[str, Field(pattern=r"^[a-z][a-z0-9_-]{0,63}$")]
Text = Annotated[str, Field(min_length=1, max_length=4000)]
EntityKind = Literal["cast", "place", "prop"]
Origin = Literal["extracted", "inferred", "proposed"]


class Record(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True, frozen=True)


class Evidence(Record):
    block: int = Field(ge=0)
    quote: Text
    occurrence: int | None = Field(default=None, ge=0, le=2000000)


class Issue(Record):
    code: Key
    scope: str = Field(min_length=1, max_length=128)
    severity: Literal["warning", "blocker"]
    summary: Text


class BlockRange(Record):
    first_block: int = Field(ge=0)
    last_block: int = Field(ge=0)

    @model_validator(mode="after")
    def ordered(self) -> Self:
        if self.first_block > self.last_block:
            raise ValueError("reversed block range")
        return self


class ExcludedBlock(BlockRange):
    kind: Literal["heading", "author_note", "non_story", "unresolved"]
    reason: Text


class Episode(BlockRange):
    key: Key
    number: int | None = Field(ge=1)
    title: Text
    rationale: Text


class EpisodeMap(Record):
    mode: Literal["preserve", "propose"]
    episodes: list[Episode] = Field(min_length=1, max_length=200)
    excluded: list[ExcludedBlock] = Field(max_length=2000)
    issues: list[Issue] = Field(max_length=200)


class Beat(Record):
    key: Key
    action: Text
    evidence: list[Evidence] = Field(min_length=1, max_length=20)
    required: bool
    origin: Origin


class Dialogue(Record):
    key: Key
    speaker_mention: Key | None
    text: Text
    evidence: Evidence
    channel: Literal["onscreen", "offscreen", "phone", "inner", "group", "unknown"]


class Mention(Record):
    key: Key
    kind: EntityKind
    name: Text
    presence: Literal["onscreen", "offscreen", "mentioned", "unknown"] = "unknown"
    evidence: Evidence
    visual_details: list[Evidence] = Field(max_length=20)


class Scene(BlockRange):
    key: Key
    title: Text
    summary: Text
    time_label: Text
    time_branch: Key
    presentation: Literal["present", "flashback", "dream", "intercut", "montage", "unknown"]
    beats: list[Beat] = Field(min_length=1, max_length=200)
    dialogues: list[Dialogue] = Field(max_length=200)
    mentions: list[Mention] = Field(max_length=300)
    issues: list[Issue] = Field(max_length=100)


class EpisodeAnalysis(Record):
    episode_key: Key
    summary: Text
    conflict: Text
    turning_point: Text
    ending_hook: Text
    scenes: list[Scene] = Field(min_length=1, max_length=200)
    excluded: list[ExcludedBlock] = Field(max_length=1000)
    issues: list[Issue] = Field(max_length=200)


class MentionRef(Record):
    episode_key: Key
    scene_key: Key
    mention_key: Key


class EntityProposal(Record):
    key: Key
    kind: EntityKind
    label: Text
    mentions: list[MentionRef] = Field(min_length=1, max_length=2000)
    identity_basis: Literal["explicit", "inferred", "uncertain"]
    evidence: list[Evidence] = Field(min_length=1, max_length=50)
    uncertainty: Text | None


class Relation(Record):
    subject: Key
    predicate: Text
    target: Key
    origin: Origin
    basis: Literal["narration", "claim", "unknown"]
    evidence: list[Evidence] = Field(min_length=1, max_length=20)


class StateEvent(Record):
    entity_key: Key
    episode_key: Key
    scene_key: Key
    time_branch: Key
    story_time: Text
    property: Text
    before: Text | None
    after: Text | None
    knowledge: Literal["known", "unknown", "conflicting"]
    basis: Literal["narration", "claim", "unknown"]
    evidence: list[Evidence] = Field(min_length=1, max_length=20)

    @model_validator(mode="after")
    def claim_is_not_known(self) -> Self:
        if self.basis != "narration" and self.knowledge == "known":
            raise ValueError("a claim or unknown basis cannot establish a known state")
        return self


class AssetNeed(Record):
    entity_key: Key
    description: Text
    evidence: list[Evidence] = Field(min_length=1, max_length=20)


class WorldBook(Record):
    entities: list[EntityProposal] = Field(max_length=2000)
    unresolved_mentions: list[MentionRef] = Field(max_length=2000)
    relations: list[Relation] = Field(max_length=2000)
    state_events: list[StateEvent] = Field(max_length=4000)
    asset_needs: list[AssetNeed] = Field(max_length=2000)
    issues: list[Issue] = Field(max_length=200)


class Blocking(Record):
    mention_key: Key
    position: Text
    facing: Text
    action: Text


class AudioCue(Record):
    dialogue_key: Key
    channel: Literal["onscreen", "offscreen", "phone", "inner", "group", "unknown"]


class Shot(Record):
    key: Key
    purpose: Text
    framing: Text
    camera_movement: Text
    action: Text
    beat_keys: list[Key] = Field(min_length=1, max_length=200)
    audio: list[AudioCue] = Field(max_length=200)
    visible_mentions: list[Key] = Field(max_length=300)
    detail_evidence: list[Evidence] = Field(max_length=100)
    duration_min_ms: int = Field(ge=100, le=120000)
    duration_max_ms: int = Field(ge=100, le=120000)
    timing_basis: Text
    screen_direction: Text
    entry_state: Text
    exit_state: Text
    panel_caption: Text

    @model_validator(mode="after")
    def duration_range(self) -> Self:
        if self.duration_min_ms > self.duration_max_ms:
            raise ValueError("reversed shot duration range")
        return self


class SceneDirection(Record):
    episode_key: Key
    scene_key: Key
    dramatic_intent: Text
    audience_knows: list[Text] = Field(max_length=100)
    withhold: list[Text] = Field(max_length=100)
    blocking: list[Blocking] = Field(max_length=300)
    shots: list[Shot] = Field(min_length=1, max_length=200)
    issues: list[Issue] = Field(max_length=200)
