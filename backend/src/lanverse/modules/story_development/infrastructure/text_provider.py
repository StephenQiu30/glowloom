from __future__ import annotations

from typing import Literal
from uuid import NAMESPACE_URL, UUID, uuid5

from langchain_core.runnables import RunnableLambda

from lanverse.modules.story_development.application.contracts.content_v1 import (
    CreativeAssetContentV1,
    SceneV1,
    ScriptContentV1,
    ShotSpecCollectionV1,
    ShotV1,
    SpeechLineV1,
)
from lanverse.modules.story_development.application.contracts.generation_v1 import (
    GeneratedCreativeAssetV1,
    StoryboardGenerationV1,
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

    async def generate_storyboard(
        self, generation_id: UUID, script_version_id: UUID, content: ScriptContentV1
    ) -> str:
        self.call_count += 1
        namespace = uuid5(NAMESPACE_URL, str(generation_id))
        asset_specs: tuple[
            tuple[Literal["character", "scene", "visual_style"], str], ...
        ] = (
            ("character", "主角"),
            ("scene", "主要场景"),
            ("visual_style", "统一画风"),
        )
        assets = tuple(
            GeneratedCreativeAssetV1(
                version_id=uuid5(namespace, f"asset-version:{asset_type}"),
                content=CreativeAssetContentV1(
                    asset_id=uuid5(namespace, f"asset:{asset_type}"),
                    asset_type=asset_type,
                    name=name,
                    description=f"{name}的统一制作设定",
                ),
            )
            for asset_type, name in asset_specs
        )
        lines = [line for scene in content.scenes for line in scene.speech_lines]
        shots = tuple(
            ShotV1.create(
                shot_id=uuid5(namespace, f"shot:{ordinal}"),
                ordinal=ordinal,
                narrative_purpose=f"推进第{ordinal}幕",
                visual_prompt=f"竖屏短剧第{ordinal}镜，保持角色场景画风一致",
                action=f"演绎第{ordinal}幕动作",
                duration_ticks=450000,
                asset_version_ids=tuple(asset.version_id for asset in assets),
                speech_line_ids=(lines[ordinal - 1].speech_line_id,),
            )
            for ordinal in range(1, 7)
        )
        storyboard = ShotSpecCollectionV1(
            script_version_id=script_version_id,
            asset_version_ids=tuple(asset.version_id for asset in assets),
            speech_line_ids=tuple(line.speech_line_id for line in lines),
            shots=shots,
        )
        return StoryboardGenerationV1(
            assets=assets, storyboard=storyboard
        ).model_dump_json()

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
