from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Literal
from uuid import UUID


@dataclass(frozen=True, slots=True)
class DeliveryVersionSnapshot:
    id: UUID
    episode_id: UUID
    version: int
    render_task_id: UUID
    render_snapshot_id: UUID
    status: Literal["rendering", "ready", "failed", "cancelled"]
    created_at: datetime
