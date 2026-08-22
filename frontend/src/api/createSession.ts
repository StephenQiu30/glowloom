// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** 此处后端没有提供注释 POST /api/sessions */
export async function createSession(
  body: API.CreateSessionRequest,
  options?: RequestOptions
) {
  return request<API.SessionResponse>("/api/sessions", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
