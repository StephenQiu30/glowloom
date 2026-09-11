from __future__ import annotations

import hashlib
import json
import sys
from pathlib import Path

import pytest
from pydantic import BaseModel

from app.reasoning.codex import (
    CodexImageInput,
    CodexMediaInvalid,
    run_codex_process,
)


class Output(BaseModel):
    value: str


def executable(path: Path, body: str) -> str:
    path.write_text(f"#!{sys.executable}\n" + body)
    path.chmod(0o700)
    return str(path)


def digest(value: bytes) -> str:
    return hashlib.sha256(value).hexdigest()


async def test_codex_process_stages_verified_images_in_canonical_order(tmp_path: Path) -> None:
    png = b"\x89PNG\r\n\x1a\n" + b"png-body"
    webp = b"RIFF" + (12).to_bytes(4, "little") + b"WEBP" + b"webp-body"
    png_path = tmp_path / "reference.png"
    webp_path = tmp_path / "reference.webp"
    png_path.write_bytes(png)
    webp_path.write_bytes(webp)
    captured = tmp_path / "captured.json"
    program = executable(
        tmp_path / "synthetic-codex",
        "import hashlib, json, pathlib, stat, sys\n"
        "images = [pathlib.Path(sys.argv[index + 1])\n"
        "          for index, value in enumerate(sys.argv) if value == '--image']\n"
        "payload = [{'name': value.name,\n"
        "            'digest': hashlib.sha256(value.read_bytes()).hexdigest(),\n"
        "            'mode': stat.S_IMODE(value.stat().st_mode)}\n"
        "           for value in images]\n"
        f"pathlib.Path({str(captured)!r}).write_text(json.dumps(payload))\n"
        "sys.stdin.buffer.read()\n"
        "target = pathlib.Path(sys.argv[sys.argv.index('--output-last-message') + 1])\n"
        'target.write_text(\'{"value":"accepted"}\')\n',
    )

    result = await run_codex_process(
        codex_bin=program,
        guidance="inspect only the attached references",
        prompt="{}",
        output_model=Output,
        timeout_seconds=5,
        image_inputs=(
            CodexImageInput(
                sort_key="reference:png",
                path=png_path,
                content_hash=digest(png),
                byte_length=len(png),
                media_type="image/png",
            ),
            CodexImageInput(
                sort_key="reference:webp",
                path=webp_path,
                content_hash=digest(webp),
                byte_length=len(webp),
                media_type="image/webp",
            ),
        ),
    )

    assert result == Output(value="accepted")
    assert json.loads(captured.read_text()) == [
        {"name": "input-000.png", "digest": digest(png), "mode": 0o400},
        {"name": "input-001.webp", "digest": digest(webp), "mode": 0o400},
    ]


@pytest.mark.parametrize("failure", ["digest", "length", "mime"])
async def test_codex_process_rejects_drifted_image_before_launch(
    tmp_path: Path,
    failure: str,
) -> None:
    content = b"\x89PNG\r\n\x1a\n" + b"png-body"
    image = tmp_path / "reference.png"
    image.write_bytes(content)
    launched = tmp_path / "launched"
    program = executable(
        tmp_path / "synthetic-codex",
        f"import pathlib\npathlib.Path({str(launched)!r}).write_text('started')\n",
    )
    declared_hash = digest(content) if failure != "digest" else digest(b"other")
    declared_length = len(content) if failure != "length" else len(content) + 1
    declared_type = "image/png" if failure != "mime" else "image/jpeg"

    with pytest.raises(CodexMediaInvalid):
        await run_codex_process(
            codex_bin=program,
            guidance="synthetic",
            prompt="{}",
            output_model=Output,
            timeout_seconds=5,
            image_inputs=(
                CodexImageInput(
                    sort_key="reference:one",
                    path=image,
                    content_hash=declared_hash,
                    byte_length=declared_length,
                    media_type=declared_type,
                ),
            ),
        )
    assert not launched.exists()


async def test_codex_process_rejects_symbolic_link_image(tmp_path: Path) -> None:
    content = b"\x89PNG\r\n\x1a\n" + b"png-body"
    target = tmp_path / "target.png"
    target.write_bytes(content)
    image = tmp_path / "reference.png"
    image.symlink_to(target)

    with pytest.raises(CodexMediaInvalid):
        await run_codex_process(
            codex_bin="codex",
            guidance="synthetic",
            prompt="{}",
            output_model=Output,
            timeout_seconds=5,
            image_inputs=(
                CodexImageInput(
                    sort_key="reference:one",
                    path=image,
                    content_hash=digest(content),
                    byte_length=len(content),
                    media_type="image/png",
                ),
            ),
        )


async def test_codex_process_rejects_noncanonical_image_order(tmp_path: Path) -> None:
    content = b"\xff\xd8\xff" + b"jpeg-body"
    first = tmp_path / "first.jpg"
    second = tmp_path / "second.jpg"
    first.write_bytes(content)
    second.write_bytes(content)

    with pytest.raises(CodexMediaInvalid):
        await run_codex_process(
            codex_bin="codex",
            guidance="synthetic",
            prompt="{}",
            output_model=Output,
            timeout_seconds=5,
            image_inputs=(
                CodexImageInput(
                    sort_key="reference:two",
                    path=second,
                    content_hash=digest(content),
                    byte_length=len(content),
                    media_type="image/jpeg",
                ),
                CodexImageInput(
                    sort_key="reference:one",
                    path=first,
                    content_hash=digest(content),
                    byte_length=len(content),
                    media_type="image/jpeg",
                ),
            ),
        )
