from __future__ import annotations

from uuid import UUID

from lanverse.infrastructure.database.pool import DatabasePool
from lanverse.modules.story_development.application.contracts.content_v1 import ScriptContentV1
from lanverse.modules.story_development.application.contracts.snapshots import (
    ScriptVersionSnapshot,
)
from lanverse.modules.story_development.infrastructure.scripts import ScriptVersionRepository


class ScriptResultRegistrar:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._scripts = ScriptVersionRepository()

    async def register(self, task_id: UUID, output_json: str) -> ScriptVersionSnapshot:
        content = ScriptContentV1.model_validate_json(output_json)
        async with self._database.transaction() as connection:
            return await self._scripts.insert_generated(connection, task_id, content)
