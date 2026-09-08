from __future__ import annotations

import hashlib

import pytest

from app.text_contract.schemas import EpisodeMap, Evidence
from app.text_contract.source import SourceEdition, inspect_source, resolve_evidence
from app.text_contract.validation import check_episode_map


def source(text: str) -> SourceEdition:
    return SourceEdition(
        revision_id="44444444-4444-4444-8444-444444444444",
        text=text,
        content_hash=hashlib.sha256(text.encode()).hexdigest(),
    )


def test_source_blocks_preserve_unicode_whitespace_and_absolute_offsets() -> None:
    value = source("第1集\r\n顾宁🙂拿钥匙。\n\n")
    blocks = inspect_source(value)
    assert "".join(block.text for block in blocks) == value.text
    assert [(block.start, block.end) for block in blocks] == [(0, 5), (5, 13), (13, 14)]
    resolved = resolve_evidence(value, Evidence(block=1, quote="🙂拿钥匙"))
    assert (resolved.start, resolved.end) == (7, 11)
    assert value.text[resolved.start : resolved.end] == "🙂拿钥匙"
    assert resolved.text_hash == hashlib.sha256("🙂拿钥匙".encode()).hexdigest()


def test_repeated_quote_requires_more_context() -> None:
    with pytest.raises(ValueError, match="ambiguous"):
        resolve_evidence(source("钥匙，又一把钥匙。"), Evidence(block=0, quote="钥匙"))
    resolved = resolve_evidence(
        source("钥匙，又一把钥匙。"), Evidence(block=0, quote="钥匙", occurrence=1)
    )
    assert (resolved.start, resolved.end) == (6, 8)
    with pytest.raises(ValueError, match="occurrence"):
        resolve_evidence(
            source("钥匙，又一把钥匙。"), Evidence(block=0, quote="钥匙", occurrence=2)
        )


def test_source_hash_is_verified_without_normalizing_text() -> None:
    with pytest.raises(ValueError, match="hash"):
        SourceEdition(
            revision_id="44444444-4444-4444-8444-444444444444",
            text="e\u0301",
            content_hash=hashlib.sha256("é".encode()).hexdigest(),
        )


def test_episode_map_cannot_skip_blocks_or_invent_missing_episode() -> None:
    value = source("第1集\n门响。\n第3集\n灯灭。\n")
    draft = EpisodeMap.model_validate(
        {
            "mode": "preserve",
            "episodes": [
                {
                    "key": "episode-one",
                    "number": 1,
                    "title": "门响",
                    "first_block": 0,
                    "last_block": 1,
                    "rationale": "原稿集号",
                },
                {
                    "key": "episode-three",
                    "number": 3,
                    "title": "灯灭",
                    "first_block": 2,
                    "last_block": 3,
                    "rationale": "原稿集号",
                },
            ],
            "excluded": [],
            "issues": [],
        }
    )
    issues = check_episode_map(value, draft)
    assert any(issue.code == "episode_number_gap" for issue in issues)
    invalid = draft.model_copy(update={"episodes": [draft.episodes[0]]})
    with pytest.raises(ValueError, match="coverage"):
        check_episode_map(value, invalid)
    wrong_number = draft.episodes[1].model_copy(update={"number": 2})
    with pytest.raises(ValueError, match="heading"):
        check_episode_map(
            value, draft.model_copy(update={"episodes": [draft.episodes[0], wrong_number]})
        )


def test_episode_map_rejects_overlap_and_headingless_preserve() -> None:
    draft = EpisodeMap.model_validate(
        {
            "mode": "preserve",
            "episodes": [
                {
                    "key": "episode-one",
                    "number": 1,
                    "title": "门响",
                    "first_block": 0,
                    "last_block": 0,
                    "rationale": "推测",
                }
            ],
            "excluded": [],
            "issues": [],
        }
    )
    with pytest.raises(ValueError, match="heading"):
        check_episode_map(source("门响。"), draft)
