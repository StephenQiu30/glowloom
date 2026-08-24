from prometheus_client import Counter, Histogram

CACHE_OPERATIONS = Counter(
    "lanverse_cache_operations_total",
    "Cache operations by registered namespace, operation and result",
    ("namespace", "operation", "result"),
)
CACHE_DURATION = Histogram(
    "lanverse_cache_operation_duration_seconds",
    "Cache operation duration by registered namespace and operation",
    ("namespace", "operation"),
)
HIGH_COST_GUARD_DECISIONS = Counter(
    "lanverse_high_cost_guard_decisions_total",
    "High cost generation guard decisions with bounded outcomes",
    ("result",),
)
HIGH_COST_GUARD_DURATION = Histogram(
    "lanverse_high_cost_guard_duration_seconds",
    "High cost generation guard decision duration",
)
