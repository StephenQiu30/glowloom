/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 顺序读取画布历史 POST /canvas/graph/checkpoint/scan */
export async function scanCanvasCheckpoints(
  body: CanvasContract.CanvasSnapshotScanRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseCanvasSnapshotSliceVO>(
    "/canvas/graph/checkpoint/scan",
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

/** 关闭画布历史浏览 POST /canvas/graph/checkpoint/scan/close */
export async function closeCanvasCheckpointScan(
  body: CanvasContract.CanvasSnapshotScanCloseRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseBoolean>(
    "/canvas/graph/checkpoint/scan/close",
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
