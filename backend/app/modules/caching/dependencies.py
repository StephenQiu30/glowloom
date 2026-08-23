from typing import cast

from fastapi import Request

from app.modules.caching.contracts import CachePort, HighCostGuardPort


def get_cache_port(request: Request) -> CachePort:
    return cast(CachePort, request.app.state.cache_port)


def get_high_cost_guard(request: Request) -> HighCostGuardPort:
    return cast(HighCostGuardPort, request.app.state.high_cost_guard)
