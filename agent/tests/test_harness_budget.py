from datetime import UTC, datetime, timedelta

import pytest

from app.harness.budget import Budget, BudgetExceeded, Usage


def test_budget_checks_worst_case_before_call_and_charges_actual_once() -> None:
    now = datetime(2026, 9, 27, tzinfo=UTC)
    budget = Budget(max_tokens=100, max_cost_micros=1000, deadline=now + timedelta(seconds=30))

    budget.before_call(prompt_tokens=20, max_output_tokens=50, max_cost_micros=700, now=now)
    budget.charge("call-1", Usage(input_tokens=20, output_tokens=30, cost_micros=500))
    budget.charge("call-1", Usage(input_tokens=20, output_tokens=30, cost_micros=500))
    assert budget.usage == Usage(input_tokens=20, output_tokens=30, cost_micros=500)

    with pytest.raises(BudgetExceeded, match="tokens"):
        budget.before_call(prompt_tokens=20, max_output_tokens=50, max_cost_micros=300, now=now)
    with pytest.raises(BudgetExceeded, match="cost"):
        budget.before_call(prompt_tokens=10, max_output_tokens=10, max_cost_micros=600, now=now)


def test_budget_rejects_deadline_and_conflicting_duplicate_charge() -> None:
    now = datetime(2026, 9, 27, tzinfo=UTC)
    budget = Budget(max_tokens=100, max_cost_micros=1000, deadline=now)
    with pytest.raises(BudgetExceeded, match="deadline"):
        budget.before_call(prompt_tokens=1, max_output_tokens=1, max_cost_micros=1, now=now)

    budget.charge("call-1", Usage(input_tokens=1, output_tokens=1, cost_micros=1))
    with pytest.raises(ValueError, match="different usage"):
        budget.charge("call-1", Usage(input_tokens=2, output_tokens=1, cost_micros=1))
