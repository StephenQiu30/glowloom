// @ts-ignore
/* eslint-disable */
import { request } from "@umijs/max";

/** 获取 Swagger 文档 GET /api/swagger.json */
export async function systemSwagger(options?: { [key: string]: any }) {
  return request<Record<string, any>>("/api/swagger.json", {
    method: "GET",
    ...(options || {}),
  });
}

/** 服务就绪检查 GET /readyz */
export async function systemReady(options?: { [key: string]: any }) {
  return request<Record<string, any>>("/readyz", {
    method: "GET",
    ...(options || {}),
  });
}
