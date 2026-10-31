/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 读取画布检索同步状态 POST /canvas/search/status */
export async function getCanvasSearchStatus(
  body: CanvasContract.CanvasSearchStatusRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseSearchVisibilityVO>(
    "/canvas/search/status",
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      data: body,
      ...(options || {}),
    },
  );
}
