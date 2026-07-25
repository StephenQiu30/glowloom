from __future__ import annotations

from uuid import NAMESPACE_URL, UUID, uuid5

from langchain_core.runnables import RunnableLambda

from lanverse.modules.story_development.application.contracts.content_v1 import (
    SceneV1,
    ScriptContentV1,
    SpeechLineV1,
)


class DeterministicTextProvider:
    def __init__(self) -> None:
        self.call_count = 0
        self._chain = RunnableLambda[dict[str, str], str](self._render_script)

    async def generate_script(self, source_revision_id: UUID, title: str) -> str:
        self.call_count += 1
        return await self._chain.ainvoke(
            {"source_revision_id": str(source_revision_id), "title": title}
        )

    @staticmethod
    def _render_script(values: dict[str, str]) -> str:
        namespace = uuid5(NAMESPACE_URL, values["source_revision_id"])
        scenes = []
        for ordinal in range(1, 7):
            line = SpeechLineV1(
                speech_line_id=uuid5(namespace, f"speech:{ordinal}"),
                ordinal=ordinal,
                kind="narration",
                text=f"第{ordinal}幕旁白",
                voice_id="narrator_female",
            )
            scenes.append(
                SceneV1(
                    scene_id=uuid5(namespace, f"scene:{ordinal}"),
                    ordinal=ordinal,
                    location=f"场景{ordinal}",
                    time_of_day="day",
                    action=f"完成第{ordinal}幕动作",
                    speech_lines=(line,),
                )
            )
        return ScriptContentV1(title=values["title"], scenes=tuple(scenes)).model_dump_json()
