from prometheus_client import CONTENT_TYPE_LATEST, Counter, Histogram, generate_latest
from starlette.middleware.base import BaseHTTPMiddleware, RequestResponseEndpoint
from starlette.requests import Request
from starlette.responses import Response

HTTP_REQUESTS = Counter(
    "lanverse_http_requests_total",
    "HTTP requests by method and status",
    ("method", "status"),
)
HTTP_DURATION = Histogram(
    "lanverse_http_request_duration_seconds",
    "HTTP request duration",
    ("method",),
)


class MetricsMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next: RequestResponseEndpoint) -> Response:
        with HTTP_DURATION.labels(method=request.method).time():
            response = await call_next(request)
        HTTP_REQUESTS.labels(method=request.method, status=str(response.status_code)).inc()
        return response


def metrics_response() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)
