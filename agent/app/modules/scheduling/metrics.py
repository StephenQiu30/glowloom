from prometheus_client import Counter, Histogram

SCHEDULE_DISPATCH_RESULTS = Counter(
    "lanverse_schedule_dispatch_total",
    "Schedule dispatch outcomes",
    ("handler", "result"),
)

SCHEDULE_MISFIRES = Counter(
    "lanverse_schedule_misfire_total",
    "Schedule occurrences outside their configured grace window",
    ("handler", "policy"),
)

SCHEDULE_LAG_SECONDS = Histogram(
    "lanverse_schedule_lag_seconds",
    "Lag between a due schedule and the PostgreSQL dispatcher clock",
    ("handler",),
    buckets=(0.1, 1, 5, 15, 30, 60, 300, 1800, 3600),
)

SCHEDULE_MANUAL_ATTENTION = Counter(
    "lanverse_schedule_manual_attention_total",
    "Schedules that stopped automatic dispatch and require operator action",
    ("handler", "reason"),
)
