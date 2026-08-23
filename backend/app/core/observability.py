import time

from prometheus_client import CONTENT_TYPE_LATEST, Counter, Histogram, generate_latest
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.requests import Request
from starlette.responses import Response

from app.core.logging import route_template

HTTP_REQUESTS = Counter(
    "lanverse_http_requests_total",
    "HTTP requests by method, route template and status class",
    ("method", "route", "status_class"),
)
HTTP_DURATION = Histogram(
    "lanverse_http_request_duration_seconds",
    "HTTP request duration by method and route template",
    ("method", "route"),
)


class MetricsMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        started = time.perf_counter()
        try:
            response = await call_next(request)
        except Exception:
            route = route_template(request)
            HTTP_DURATION.labels(method=request.method, route=route).observe(
                time.perf_counter() - started
            )
            HTTP_REQUESTS.labels(
                method=request.method,
                route=route,
                status_class="5xx",
            ).inc()
            raise
        route = route_template(request)
        HTTP_DURATION.labels(method=request.method, route=route).observe(
            time.perf_counter() - started
        )
        HTTP_REQUESTS.labels(
            method=request.method,
            route=route,
            status_class=f"{response.status_code // 100}xx",
        ).inc()
        return response


def metrics_response() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)
