import json
from dataclasses import dataclass
from hashlib import sha256
from uuid import UUID

from app.modules.storyboards.schemas import AssetReferenceRequest, ShotSpec


@dataclass(frozen=True, slots=True)
class StoryboardHashes:
    content_hash: str
    input_hash: str


def _canonical_hash(payload: object) -> str:
    canonical = json.dumps(
        payload,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return sha256(canonical.encode()).hexdigest()


def storyboard_content_hashes(
    spec: ShotSpec,
    references: list[AssetReferenceRequest],
) -> StoryboardHashes:
    normalized_spec = spec.model_dump(mode="json")
    normalized_references = sorted(
        (reference.model_dump(mode="json") for reference in references),
        key=lambda reference: reference["slot_key"],
    )
    content_hash = _canonical_hash(
        {
            "spec": normalized_spec,
            "asset_references": normalized_references,
        }
    )
    input_hash = _canonical_hash(
        {
            "content_hash": content_hash,
            "schema_version": spec.schema_version,
            "script_reference": spec.script_reference.model_dump(mode="json"),
        }
    )
    return StoryboardHashes(content_hash=content_hash, input_hash=input_hash)


def shot_order_hash(shot_ids: list[UUID]) -> str:
    return _canonical_hash([str(shot_id) for shot_id in shot_ids])
