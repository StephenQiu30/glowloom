from __future__ import annotations

from schemas.rendering import RenderRecipeV1


class RenderRuntimeUnavailable(RuntimeError):
    pass


def pinned_render_recipe(runtime_image: str | None) -> RenderRecipeV1:
    if runtime_image is None:
        raise RenderRuntimeUnavailable("LANVERSE_RENDER_RUNTIME_IMAGE is required")
    return RenderRecipeV1(
        runtime_image=runtime_image,
        ffmpeg_version="8.1",
        ffprobe_version="8.1",
        font_name="Noto Sans CJK SC",
        font_file="/usr/share/fonts/opentype/noto/NotoSansCJKsc-Regular.otf",
        font_sha256="2c76254f6fc379fddfce0a7e84fb5385bb135d3e399294f6eeb6680d0365b74b",
        font_license="OFL-1.1",
    )
