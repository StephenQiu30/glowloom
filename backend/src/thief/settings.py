from __future__ import annotations

import os


LOCAL_DATABASE_URL = "postgresql+psycopg://thief:thief_local@localhost:5432/thief"
LOCAL_RABBITMQ_URL = "amqp://thief:thief_local@localhost:5672//"


def database_url() -> str:
    return _required_in_production("THIEF_DATABASE_URL", LOCAL_DATABASE_URL)


def rabbitmq_url() -> str:
    return _required_in_production("THIEF_RABBITMQ_URL", LOCAL_RABBITMQ_URL)


def _required_in_production(name: str, default: str) -> str:
    value = os.getenv(name)
    if os.getenv("THIEF_ENV", "development") == "production" and not value:
        raise RuntimeError(f"{name} is required in production")
    return value or default
