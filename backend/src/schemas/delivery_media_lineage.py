from __future__ import annotations

import json
from collections.abc import Mapping
from typing import Any

from pydantic import ValidationError

from schemas.delivery_manifest import DeliveryMediaLineageV1


class DeliveryMediaLineageInvalid(ValueError):
    pass


def media_lineage(row: Mapping[str, Any]) -> DeliveryMediaLineageV1:
    probe = row["probe_summary_json"]
    if isinstance(probe, str):
        probe = json.loads(probe)
    if not isinstance(probe, dict):
        raise DeliveryMediaLineageInvalid("media probe lineage is incomplete")
    try:
        return DeliveryMediaLineageV1(
            usage_type=row["usage_type"],
            usage_id=row["usage_id"],
            input_version_id=row["input_version_id"],
            input_hash=row["input_hash"],
            adoption_id=row["adoption_id"],
            candidate_id=row["candidate_id"],
            media_version_id=row["media_version_id"],
            media_sha256=row["media_sha256"],
            media_kind=row["media_kind"],
            source_kind=row["source_kind"],
            mime_type=row["mime_type"],
            byte_size=row["byte_size"],
            duration_ticks=row["duration_ticks"],
            timebase=row["timebase"],
            probe_summary={str(key): item for key, item in probe.items()},
            origin_attempt_id=row["origin_attempt_id"],
            origin_task_id=row["origin_task_id"],
            origin_submission_snapshot_id=row["origin_submission_snapshot_id"],
            capability=row["capability"],
            model_profile_id=row["model_profile_id"],
            provider_id=row["provider_id"],
            model_id=row["model_id"],
            route_version=row["route_version"],
            provider_schema_version=row["schema_version"],
        )
    except (KeyError, TypeError, ValidationError) as error:
        raise DeliveryMediaLineageInvalid("media provider lineage is incomplete") from error
