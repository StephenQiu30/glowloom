// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/**  Healthz GET /healthz */
export async function healthzHealthzGet(options?: RequestOptions) {
  return request<API.HealthResponse>("/healthz", {
    method: "GET",
    ...(options || {}),
  });
}

/**  Metrics GET /metrics */
export async function metricsMetricsGet(options?: RequestOptions) {
  return request<any>("/metrics", {
    method: "GET",
    ...(options || {}),
  });
}

/**  Readyz GET /readyz */
export async function readyzReadyzGet(options?: RequestOptions) {
  return request<API.ReadinessResponse>("/readyz", {
    method: "GET",
    ...(options || {}),
  });
}
