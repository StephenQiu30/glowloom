import json
from hashlib import sha256
from uuid import UUID


def canonical_hash(payload: object) -> str:
    canonical = json.dumps(
        payload,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    )
    return sha256(canonical.encode()).hexdigest()


def structure_hash(
    *,
    script_version_id: UUID,
    revision: int,
    units: list[dict[str, object]],
) -> str:
    return canonical_hash(
        {
            "script_version_id": str(script_version_id),
            "revision": revision,
            "units": units,
        }
    )


def dependency_hash(
    *,
    script_version_id: UUID,
    structure_hash_value: str,
    unit_version_ids: list[UUID],
) -> str:
    return canonical_hash(
        {
            "script_version_id": str(script_version_id),
            "structure_hash": structure_hash_value,
            "unit_version_ids": [str(item) for item in unit_version_ids],
        }
    )
