/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 预览当前可见画布固定媒体 POST /canvas/media/read */
export async function readCanvasMedia(
  body: CanvasContract.FileCanvasReadRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseFileSignedUrlVO>(
    "/canvas/media/read",
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
