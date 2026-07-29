// @ts-ignore
/* eslint-disable */
import request, { type RequestOptions } from "@/lib/request";

/** List Projects GET /api/v1/projects */
export async function listProjectsApiV1ProjectsGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listProjectsApiV1ProjectsGetParams,
  options?: RequestOptions
) {
  return request<API.ApiResponsePaginatedProjects_>("/api/v1/projects", {
    method: "GET",
    params: {
      ...params,
    },
    ...(options || {}),
  });
}

/** Create Project POST /api/v1/projects */
export async function createProjectApiV1ProjectsPost(
  body: API.ProjectCreateRequest,
  options?: RequestOptions
) {
  return request<API.ApiResponseProjectResponse_>("/api/v1/projects", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    data: body,
    ...(options || {}),
  });
}

/** Get Project GET /api/v1/projects/${param0} */
export async function getProjectApiV1ProjectsProjectIdGet(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getProjectApiV1ProjectsProjectIdGetParams,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseProjectResponse_>(
    `/api/v1/projects/${param0}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Delete Project DELETE /api/v1/projects/${param0} */
export async function deleteProjectApiV1ProjectsProjectIdDelete(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteProjectApiV1ProjectsProjectIdDeleteParams,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseDeleteResponse_>(`/api/v1/projects/${param0}`, {
    method: "DELETE",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** Update Project PATCH /api/v1/projects/${param0} */
export async function updateProjectApiV1ProjectsProjectIdPatch(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateProjectApiV1ProjectsProjectIdPatchParams,
  body: API.ProjectUpdateRequest,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseProjectResponse_>(
    `/api/v1/projects/${param0}`,
    {
      method: "PATCH",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    }
  );
}

/** Archive Project POST /api/v1/projects/${param0}/archive */
export async function archiveProjectApiV1ProjectsProjectIdArchivePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.archiveProjectApiV1ProjectsProjectIdArchivePostParams,
  body: API.ProjectStateRequest,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseProjectResponse_>(
    `/api/v1/projects/${param0}/archive`,
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

/** Update Budget Limit POST /api/v1/projects/${param0}/budget-limit */
export async function updateBudgetLimitApiV1ProjectsProjectIdBudgetLimitPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateBudgetLimitApiV1ProjectsProjectIdBudgetLimitPostParams,
  body: API.BudgetLimitRequest,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseProjectResponse_>(
    `/api/v1/projects/${param0}/budget-limit`,
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

/** Delete Preflight POST /api/v1/projects/${param0}/delete-preflight */
export async function deletePreflightApiV1ProjectsProjectIdDeletePreflightPost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deletePreflightApiV1ProjectsProjectIdDeletePreflightPostParams,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseDeletePreflightResponse_>(
    `/api/v1/projects/${param0}/delete-preflight`,
    {
      method: "POST",
      params: { ...queryParams },
      ...(options || {}),
    }
  );
}

/** Restore Project POST /api/v1/projects/${param0}/restore */
export async function restoreProjectApiV1ProjectsProjectIdRestorePost(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.restoreProjectApiV1ProjectsProjectIdRestorePostParams,
  body: API.ProjectStateRequest,
  options?: RequestOptions
) {
  const { project_id: param0, ...queryParams } = params;
  return request<API.ApiResponseProjectResponse_>(
    `/api/v1/projects/${param0}/restore`,
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
