from typing import cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.scripts.contracts import (
    ProductionBibleEntityInput,
    ProductionBibleExtractionInput,
    ProductionBibleStateInput,
    ProductionBibleWorldInput,
)
from app.modules.scripts.production_bibles import repository
from app.modules.scripts.production_bibles.models import ProductionBibleEntityState


async def resolve_production_bible_context(
    session: AsyncSession,
    *,
    bible_id: UUID,
    revision: int,
    result_hash: str,
    episode_number: int,
) -> ProductionBibleExtractionInput | None:
    """Resolve one immutable, episode-scoped view of a confirmed Bible."""

    bible = await repository.find_bible(session, bible_id)
    if (
        bible is None
        or bible.status != "confirmed"
        or bible.revision != revision
        or bible.result_hash != result_hash
    ):
        return None
    entities = await repository.list_entities(session, bible.id)
    states = await repository.list_entity_states(session, bible.id)
    states_by_entity: dict[UUID, list[ProductionBibleEntityState]] = {}
    for state in states:
        states_by_entity.setdefault(state.entity_id, []).append(state)
    entity_inputs: list[ProductionBibleEntityInput] = []
    for entity in entities:
        if entity.episode_numbers and episode_number not in entity.episode_numbers:
            continue
        state_inputs: list[ProductionBibleStateInput] = []
        for state in states_by_entity.get(entity.id, []):
            if state.episode_numbers and episode_number not in state.episode_numbers:
                continue
            if state.asset_state_id is None or state.asset_version_id is None:
                return None
            state_inputs.append(
                ProductionBibleStateInput(
                    entity_key=entity.entity_key,
                    state_key=state.state_key,
                    label=state.label,
                    asset_state_id=state.asset_state_id,
                    asset_version_id=state.asset_version_id,
                    state_spec=state.state_spec,
                )
            )
        entity_inputs.append(
            ProductionBibleEntityInput(
                entity_key=entity.entity_key,
                kind=entity.kind,
                canonical_name=entity.canonical_name,
                aliases=tuple(entity.aliases),
                stable_spec=entity.stable_spec,
                states=tuple(state_inputs),
            )
        )
    world_entries = await repository.list_world_entries(session, bible.id)
    return ProductionBibleExtractionInput(
        bible_id=bible.id,
        revision=bible.revision,
        result_hash=cast(str, bible.result_hash),
        entities=tuple(entity_inputs),
        world_entries=tuple(
            ProductionBibleWorldInput(
                entry_key=entry.entry_key,
                category=entry.category,
                title=entry.title,
                facts=tuple(entry.facts),
                rules=tuple(entry.rules),
                entity_keys=tuple(entry.entity_keys),
            )
            for entry in world_entries
            if not entry.episode_numbers or episode_number in entry.episode_numbers
        ),
    )
