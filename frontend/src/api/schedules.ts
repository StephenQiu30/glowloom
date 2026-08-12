// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** List Schedules GET /api/v1/schedules */
export async function listSchedulesApiV1SchedulesGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listSchedulesApiV1SchedulesGetParams,
  options?: RequestOptions
) {
  return request<API.ApiResponsePaginatedSchedules_>("/api/v1/schedules", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Configure Schedule PUT /api/v1/schedules/${param0}/configuration */
export async function configureScheduleApiV1SchedulesScheduleIdConfigurationPut(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.configureScheduleApiV1SchedulesScheduleIdConfigurationPutParams,
  body: API.ScheduleConfigurationRequest,
  options?: RequestOptions
) {
  const { schedule_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScheduleResponse_>(
    `/api/v1/schedules/${param0}/configuration`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Pause Schedule POST /api/v1/schedules/${param0}/pause */
export async function pauseScheduleApiV1SchedulesScheduleIdPausePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.pauseScheduleApiV1SchedulesScheduleIdPausePostParams,
  body: API.ScheduleStateRequest,
  options?: RequestOptions
) {
  const { schedule_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScheduleResponse_>(
    `/api/v1/schedules/${param0}/pause`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Resume Schedule POST /api/v1/schedules/${param0}/resume */
export async function resumeScheduleApiV1SchedulesScheduleIdResumePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.resumeScheduleApiV1SchedulesScheduleIdResumePostParams,
  body: API.ScheduleResumeRequest,
  options?: RequestOptions
) {
  const { schedule_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScheduleResponse_>(
    `/api/v1/schedules/${param0}/resume`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Trigger Schedule POST /api/v1/schedules/${param0}/trigger */
export async function triggerScheduleApiV1SchedulesScheduleIdTriggerPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.triggerScheduleApiV1SchedulesScheduleIdTriggerPostParams,
  body: API.ScheduleTriggerRequest,
  options?: RequestOptions
) {
  const { schedule_id: param0, ...queryParams } = params;
  return request<API.ApiResponseScheduleFireResponse_>(
    `/api/v1/schedules/${param0}/trigger`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}
