import pytest

from app.core.migrations import (
    DatabaseRevisionError,
    ensure_expected_heads,
    validate_backup_reference,
)


def test_revision_gate_accepts_all_expected_heads_regardless_of_order() -> None:
    ensure_expected_heads(("head_b", "head_a"), ("head_a", "head_b"))


@pytest.mark.parametrize(
    ("current", "expected"),
    [
        ((), ("head_a",)),
        (("old",), ("head_a",)),
        (("head_a",), ("head_a", "head_b")),
    ],
)
def test_revision_gate_rejects_unversioned_outdated_and_partial_heads(
    current: tuple[str, ...],
    expected: tuple[str, ...],
) -> None:
    with pytest.raises(DatabaseRevisionError, match="not at migration head"):
        ensure_expected_heads(current, expected)


def test_existing_database_adoption_requires_backup_reference() -> None:
    for value in ("   ", "backup with spaces", "backup\nforged", "b" * 201):
        with pytest.raises(ValueError, match="backup reference is required"):
            validate_backup_reference(value)

    assert validate_backup_reference(" backup-20260813-001 ") == "backup-20260813-001"
