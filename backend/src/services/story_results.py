from __future__ import annotations

from uuid import UUID

from db.pool import DatabasePool
from repositories.scripts import ScriptVersionRepository
from repositories.storyboards import (
    StoryboardGenerationRepository,
)
from schemas.story_content import ScriptContentV1
from schemas.story_generation import (
    StoryboardGenerationV1,
)
from schemas.story_snapshots import (
    ScriptVersionSnapshot,
    StoryboardGenerationSnapshot,
)


class ScriptResultRegistrar:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._scripts = ScriptVersionRepository()

    async def register(self, task_id: UUID, output_json: str) -> ScriptVersionSnapshot:
        content = ScriptContentV1.model_validate_json(output_json)
        async with self._database.transaction() as connection:
            return await self._scripts.insert_generated(connection, task_id, content)


class StoryboardResultRegistrar:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._storyboards = StoryboardGenerationRepository()

    async def register(self, task_id: UUID, output_json: str) -> StoryboardGenerationSnapshot:
        content = StoryboardGenerationV1.model_validate_json(output_json)
        async with self._database.transaction() as connection:
            return await self._storyboards.insert_generated(connection, task_id, content)
