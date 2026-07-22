from __future__ import annotations

import argparse
import json

from celery import Celery  # type: ignore[import-untyped]

from thief_worker.settings import WorkerSettings


settings = WorkerSettings.from_env()
app = Celery(
    "thief",
    broker=settings.rabbitmq_url,
)
app.conf.task_default_queue = "generation"
app.conf.worker_enable_remote_control = False


def health() -> dict[str, str]:
    return {"service": "worker", "status": "ok"}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--healthcheck", action="store_true")
    arguments = parser.parse_args(argv)
    if arguments.healthcheck:
        print(json.dumps(health()))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
