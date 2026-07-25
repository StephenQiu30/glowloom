from __future__ import annotations

import asyncio


class WorkerCapacity:
    def __init__(self, *, limit: int) -> None:
        if limit < 1:
            raise ValueError("capacity limit must be positive")
        self._limit = limit
        self._active = 0
        self._lock = asyncio.Lock()

    @property
    def active(self) -> int:
        return self._active

    async def try_acquire(self) -> bool:
        async with self._lock:
            if self._active >= self._limit:
                return False
            self._active += 1
            return True

    async def release(self) -> None:
        async with self._lock:
            if self._active < 1:
                raise RuntimeError("capacity release without an active slot")
            self._active -= 1
