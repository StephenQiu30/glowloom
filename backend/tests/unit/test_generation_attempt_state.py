import pytest
from uuid6 import uuid7

from app.modules.production.generation_execution import (
    generation_provider_request_key,
)


def test_provider_request_key_is_stable_bounded_and_attempt_specific() -> None:
    workspace_id = uuid7()
    task_id = uuid7()
    first = generation_provider_request_key(
        workspace_id=workspace_id,
        task_id=task_id,
        sequence=1,
        input_hash="a" * 64,
    )

    assert first == generation_provider_request_key(
        workspace_id=workspace_id,
        task_id=task_id,
        sequence=1,
        input_hash="a" * 64,
    )
    assert len(first) == 64
    assert str(workspace_id) not in first
    assert str(task_id) not in first
    assert first != generation_provider_request_key(
        workspace_id=workspace_id,
        task_id=task_id,
        sequence=2,
        input_hash="a" * 64,
    )
    assert first != generation_provider_request_key(
        workspace_id=workspace_id,
        task_id=task_id,
        sequence=1,
        input_hash="b" * 64,
    )


def test_provider_request_key_rejects_non_positive_sequence() -> None:
    with pytest.raises(ValueError, match="sequence must be positive"):
        generation_provider_request_key(
            workspace_id=uuid7(),
            task_id=uuid7(),
            sequence=0,
            input_hash="a" * 64,
        )
