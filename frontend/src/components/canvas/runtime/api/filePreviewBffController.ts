/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 请求固定源版本的小图或封面 POST /files/preview */
export async function previewFile(
  body: CanvasContract.FilePreviewRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseFilePreviewVO>("/files/preview", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 批量请求最多48个可见素材预览 POST /files/previews */
export async function previewFiles(
  body: CanvasContract.FilePreviewRequest[],
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseListFilePreviewVO>(
    "/files/previews",
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
