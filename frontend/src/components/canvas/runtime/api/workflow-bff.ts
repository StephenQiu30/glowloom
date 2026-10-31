/* eslint-disable */
import { request } from "@/components/canvas/runtime/lib/request";
import type { AxiosRequestConfig } from "axios";

/** 读取原流程命令回执 GET /workflow/command */
export async function getWorkflowCommand(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getWorkflowCommandParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseWorkflowRunVO>(
    "/workflow/command",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 读取本人流程完整状态 GET /workflow/get */
export async function getWorkflowRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.getWorkflowRunParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseWorkflowRunVO>("/workflow/get", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** 读取画布原运行结果 GET /workflow/history */
export async function listWorkflowHistory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: CanvasContract.listWorkflowHistoryParams,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseListWorkflowRunVO>(
    "/workflow/history",
    {
      method: "GET",
      params: {
        ...params,
      },
      ...(options || {}),
    },
  );
}

/** 读取流程可运行能力 GET /workflow/options */
export async function getWorkflowOptions(options?: AxiosRequestConfig) {
  return request<CanvasContract.BaseResponseWorkflowOptionsVO>(
    "/workflow/options",
    {
      method: "GET",
      ...(options || {}),
    },
  );
}

/** 停止新节点准入并等待原在途回执 POST /workflow/stop */
export async function stopWorkflowRun(
  body: CanvasContract.WorkflowStopRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseWorkflowRunVO>("/workflow/stop", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** 关闭未受理流程提交或返回原运行 POST /workflow/submission/close */
export async function closeWorkflowSubmission(
  body: CanvasContract.WorkflowCommandCloseRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseWorkflowSubmissionVO>(
    "/workflow/submission/close",
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

/** 运行已保存画布的选中范围 POST /workflow/submit */
export async function submitWorkflowRun(
  body: CanvasContract.WorkflowSubmitRequest,
  options?: AxiosRequestConfig,
) {
  return request<CanvasContract.BaseResponseWorkflowRunVO>("/workflow/submit", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}
