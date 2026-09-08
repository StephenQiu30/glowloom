from __future__ import annotations

import hashlib
import re
from typing import Annotated, Self
from uuid import UUID

from pydantic import Field, field_validator, model_validator

from app.text_contract.schemas import Evidence, Record

Digest = Annotated[str, Field(pattern=r"^[a-f0-9]{64}$")]


class SourceEdition(Record):
    revision_id: str
    content_hash: Digest
    text: str = Field(min_length=1, max_length=2000000)

    @field_validator("revision_id")
    @classmethod
    def canonical_uuid(cls, value: str) -> str:
        identifier = UUID(value)
        if str(identifier) != value or identifier.int == 0:
            raise ValueError("source revision requires a canonical non-nil UUID")
        return value

    @model_validator(mode="after")
    def verify_hash(self) -> Self:
        if hashlib.sha256(self.text.encode("utf-8")).hexdigest() != self.content_hash:
            raise ValueError("source hash mismatch")
        if not self.text.strip():
            raise ValueError("source is blank")
        return self


class SourceBlock(Record):
    index: int
    start: int
    end: int
    text: str
    heading_number: int | None


class ResolvedEvidence(Record):
    revision_id: str
    source_hash: Digest
    block: int
    start: int
    end: int
    text_hash: Digest
    quote: str


_HEADING = re.compile(
    r"^\s*(?:第\s*([0-9零〇一二两三四五六七八九十百千]+)\s*集|EP(?:ISODE)?\s*(\d+))(?:\s|[:：.、-]|$)",
    re.IGNORECASE,
)
_DIGITS = {char: value for value, char in enumerate("零一二三四五六七八九")}
_DIGITS.update({"〇": 0, "两": 2})


def _number(value: str) -> int:
    if value.isascii() and value.isdigit():
        return int(value)
    total, current = 0, 0
    for char in value:
        if char in _DIGITS:
            current = _DIGITS[char]
        else:
            total += (current or 1) * {"十": 10, "百": 100, "千": 1000}[char]
            current = 0
    return total + current


def inspect_source(source: SourceEdition) -> list[SourceBlock]:
    result: list[SourceBlock] = []
    start = 0
    for index, text in enumerate(source.text.splitlines(keepends=True)):
        match = _HEADING.match(text)
        result.append(
            SourceBlock(
                index=index,
                start=start,
                end=start + len(text),
                text=text,
                heading_number=_number(next(group for group in match.groups() if group))
                if match
                else None,
            )
        )
        start += len(text)
    return result


def resolve_evidence(
    source: SourceEdition, evidence: Evidence, blocks: list[SourceBlock] | None = None
) -> ResolvedEvidence:
    blocks = blocks if blocks is not None else inspect_source(source)
    if evidence.block >= len(blocks):
        raise ValueError("evidence block is outside the source")
    block = blocks[evidence.block]
    offset = block.text.find(evidence.quote)
    if offset < 0:
        raise ValueError("evidence quote differs from the source")
    if evidence.occurrence is not None:
        for _ in range(evidence.occurrence):
            offset = block.text.find(evidence.quote, offset + 1)
            if offset < 0:
                raise ValueError("evidence occurrence is outside the source block")
    elif block.text.find(evidence.quote, offset + 1) >= 0:
        raise ValueError("ambiguous evidence quote; include more source context")
    return ResolvedEvidence(
        revision_id=source.revision_id,
        source_hash=source.content_hash,
        block=evidence.block,
        start=block.start + offset,
        end=block.start + offset + len(evidence.quote),
        text_hash=hashlib.sha256(evidence.quote.encode()).hexdigest(),
        quote=evidence.quote,
    )
