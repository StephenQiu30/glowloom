from app.modules.scripts.extractions.schemas import ScriptExtractionResult
from app.modules.scripts.extractions.service import ambiguous_occurrence_candidate_keys


def _occurrence(
    candidate_key: str,
    *,
    entity_key: str,
    state_key: str,
    scene_candidate_key: str,
    source_start: int,
) -> dict[str, object]:
    return {
        "candidate_key": candidate_key,
        "source_range": {"start": source_start, "end": source_start + 1},
        "proposal": {
            "kind": "asset_occurrence",
            "entity_key": entity_key,
            "state_key": state_key,
            "scene_candidate_key": scene_candidate_key,
            "role": "location",
        },
    }


def _scene(candidate_key: str, source_start: int) -> dict[str, object]:
    return {
        "candidate_key": candidate_key,
        "source_range": {"start": source_start, "end": source_start + 10},
        "proposal": {
            "kind": "scene",
            "heading": candidate_key,
            "location": candidate_key,
            "time_of_day": "DAY",
            "summary": candidate_key,
        },
    }


def test_only_multiple_states_for_same_entity_and_scene_require_review() -> None:
    result = ScriptExtractionResult.model_validate(
        {
            "candidates": [
                _scene("scene-003", 0),
                _scene("scene-004", 10),
                _occurrence(
                    "sky-city-base",
                    entity_key="location:sky_city",
                    state_key="base",
                    scene_candidate_key="scene-003",
                    source_start=1,
                ),
                _occurrence(
                    "sky-city-storm",
                    entity_key="location:sky_city",
                    state_key="storm_strike",
                    scene_candidate_key="scene-003",
                    source_start=2,
                ),
                _occurrence(
                    "sky-city-next-scene",
                    entity_key="location:sky_city",
                    state_key="base",
                    scene_candidate_key="scene-004",
                    source_start=11,
                ),
                _occurrence(
                    "estate-base",
                    entity_key="location:sterling_estate",
                    state_key="base",
                    scene_candidate_key="scene-003",
                    source_start=3,
                ),
            ]
        }
    )

    assert ambiguous_occurrence_candidate_keys(result) == {
        "sky-city-base",
        "sky-city-storm",
    }
