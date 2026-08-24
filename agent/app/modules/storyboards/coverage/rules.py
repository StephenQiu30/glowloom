from typing import Literal


def required_channel(kind: str) -> Literal["visual", "audio"]:
    return "audio" if kind in {"dialogue", "narration"} else "visual"
