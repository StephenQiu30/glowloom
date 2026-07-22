from __future__ import annotations

import os
from dataclasses import dataclass


LOCAL_DATABASE_URL = "postgresql+psycopg://thief:thief_local@localhost:5432/thief"


@dataclass(frozen=True, slots=True)
class ApiSettings:
    database_url: str

    @classmethod
    def from_env(cls) -> ApiSettings:
        environment = os.getenv("THIEF_ENV", "development")
        database_url = os.getenv("THIEF_DATABASE_URL")
        if environment == "production" and not database_url:
            raise RuntimeError("THIEF_DATABASE_URL is required in production")
        return cls(database_url=database_url or LOCAL_DATABASE_URL)
