import os

from celery import Celery  # type: ignore[import-untyped]


app = Celery(
    "thief",
    broker=os.getenv("THIEF_RABBITMQ_URL", "amqp://guest:guest@localhost:5672//"),
)
app.conf.task_default_queue = "generation"


def health() -> dict[str, str]:
    return {"service": "worker", "status": "ok"}
