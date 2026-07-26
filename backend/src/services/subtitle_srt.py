from __future__ import annotations

from schemas.subtitles import SubtitleContentV1


def _timestamp(ticks: int) -> str:
    milliseconds = (ticks + 45) // 90
    hours, remainder = divmod(milliseconds, 3_600_000)
    minutes, remainder = divmod(remainder, 60_000)
    seconds, millis = divmod(remainder, 1_000)
    return f"{hours:02d}:{minutes:02d}:{seconds:02d},{millis:03d}"


def render_srt(content: SubtitleContentV1) -> str:
    blocks = (
        f"{cue.ordinal}\n{_timestamp(cue.start_ticks)} --> "
        f"{_timestamp(cue.end_ticks)}\n{cue.text}"
        for cue in content.cues
    )
    return "\n\n".join(blocks) + "\n"
