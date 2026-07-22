from __future__ import annotations

import os
from dataclasses import dataclass


LOCAL_BROKER_URL = "amqp://thief:thief_local@localhost:5672//"


@dataclass(frozen=True)
class SchedulerSettings:
    rabbitmq_url: str

    @classmethod
    def from_env(cls) -> SchedulerSettings:
        environment = os.getenv("THIEF_ENV", "development")
        rabbitmq_url = os.getenv("THIEF_RABBITMQ_URL")
        if environment == "production" and not rabbitmq_url:
            raise RuntimeError("THIEF_RABBITMQ_URL is required in production")

        return cls(rabbitmq_url=rabbitmq_url or LOCAL_BROKER_URL)
