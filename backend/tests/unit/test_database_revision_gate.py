import pytest

from app.core.migrations import DatabaseRevisionError, ensure_expected_heads


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
