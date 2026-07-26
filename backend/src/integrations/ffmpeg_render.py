from __future__ import annotations

import asyncio
import os
import subprocess
import tempfile
from pathlib import Path

from integrations.ffmpeg_recipe import RenderSources, build_ffmpeg_arguments
from schemas.rendering import RenderRecipeV1

MAX_OUTPUT_BYTES = 512 * 1024 * 1024


class RenderRuntimeError(RuntimeError):
    pass


class DockerRenderRuntime:
    def __init__(self, *, timeout_seconds: float = 120) -> None:
        self._timeout_seconds = timeout_seconds

    async def render(self, sources: RenderSources, recipe: RenderRecipeV1) -> bytes:
        await self._verify_recipe(recipe)
        with tempfile.TemporaryDirectory(prefix="lanverse-render-") as directory:
            work = Path(directory)
            for index, video_source in enumerate(sources.videos):
                (work / f"video-{index:02d}.mp4").write_bytes(video_source.data)
            for index, audio_source in enumerate(sources.audios):
                (work / f"audio-{index:03d}.wav").write_bytes(audio_source.data)
            (work / "subtitles.srt").write_text(sources.subtitles_srt, encoding="utf-8")
            await self._run(
                recipe.runtime_image,
                "ffmpeg",
                build_ffmpeg_arguments(sources, recipe),
                mount=work,
            )
            output = (work / "output.mp4").read_bytes()
        if not output or len(output) > MAX_OUTPUT_BYTES:
            raise RenderRuntimeError("FFmpeg output size is invalid")
        return output

    async def _verify_recipe(self, recipe: RenderRecipeV1) -> None:
        ffmpeg = await self._run(recipe.runtime_image, "ffmpeg", ("-version",))
        ffprobe = await self._run(recipe.runtime_image, "ffprobe", ("-version",))
        font = await self._run(recipe.runtime_image, "sha256sum", (recipe.font_file,))
        if not ffmpeg.startswith(f"ffmpeg version {recipe.ffmpeg_version} ".encode()):
            raise RenderRuntimeError("FFmpeg version does not match the recipe")
        if not ffprobe.startswith(f"ffprobe version {recipe.ffprobe_version} ".encode()):
            raise RenderRuntimeError("ffprobe version does not match the recipe")
        if font.decode().split(maxsplit=1)[0] != recipe.font_sha256:
            raise RenderRuntimeError("font digest does not match the recipe")

    async def _run(
        self,
        image: str,
        entrypoint: str,
        arguments: tuple[str, ...],
        *,
        mount: Path | None = None,
    ) -> bytes:
        options = (
            "docker",
            "run",
            "--rm",
            "--pull=missing",
            "--network",
            "none",
            "--read-only",
            "--cap-drop",
            "ALL",
            "--security-opt",
            "no-new-privileges",
            "--pids-limit",
            "128",
            "--cpus",
            "2",
            "--memory",
            "1g",
            "--tmpfs",
            "/tmp:rw,noexec,nosuid,size=64m",
            "--env",
            "HOME=/tmp",
            "--user",
            f"{os.getuid()}:{os.getgid()}",
            "--entrypoint",
            entrypoint,
        )
        volume = ("--mount", f"type=bind,src={mount},dst=/work") if mount else ()
        command = (*options, *volume, image, *arguments)
        try:
            result = await asyncio.to_thread(
                subprocess.run,
                command,
                capture_output=True,
                check=False,
                timeout=self._timeout_seconds,
            )
        except (OSError, subprocess.TimeoutExpired) as error:
            raise RenderRuntimeError("FFmpeg runtime is unavailable") from error
        if result.returncode != 0:
            detail = result.stderr.decode(errors="replace")[-1000:].strip()
            raise RenderRuntimeError(f"FFmpeg command failed: {detail}")
        return result.stdout
