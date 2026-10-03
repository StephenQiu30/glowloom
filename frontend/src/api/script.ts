// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";

/** 读取分集规范正文区间 GET /api/episodes/${param0}/source-text */
export async function getScriptEpisodeSourceText(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptEpisodeSourceTextParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { eid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationEpisodeText>(
    `/api/episodes/${param0}/source-text`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 当前手工结构 GET /api/episodes/${param0}/structure */
export async function getScriptEpisodeStructure(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptEpisodeStructureParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { eid: param0, ...queryParams } = params;
  return request<API.internalScriptAdapterHttpStructureDetail>(
    `/api/episodes/${param0}/structure`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 保存完整手工结构候选 POST /api/episodes/${param0}/structure */
export async function saveScriptEpisodeStructure(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.saveScriptEpisodeStructureParams,
  body: API.internalScriptAdapterHttpStructureSaveRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { eid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationStructureReceipt>(
    `/api/episodes/${param0}/structure`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 明确确认当前手工结构 POST /api/episodes/${param0}/structure/confirm */
export async function confirmScriptEpisodeStructure(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmScriptEpisodeStructureParams,
  body: API.internalScriptAdapterHttpStructureConfirmRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { eid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationStructureReceipt>(
    `/api/episodes/${param0}/structure/confirm`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 手工结构全部版本 GET /api/episodes/${param0}/structure/versions */
export async function listScriptEpisodeStructureVersions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listScriptEpisodeStructureVersionsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { eid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationStructurePage>(
    `/api/episodes/${param0}/structure/versions`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 指定历史手工结构 GET /api/episodes/${param0}/structure/versions/${param1} */
export async function getScriptEpisodeStructureVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptEpisodeStructureVersionParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { eid: param0, version: param1, ...queryParams } = params;
  return request<API.internalScriptAdapterHttpStructureDetail>(
    `/api/episodes/${param0}/structure/versions/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 指定版本的候选与正式分集 GET /api/projects/${param0}/episodes */
export async function listScriptEpisodes(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listScriptEpisodesParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationEpisodeView>(
    `/api/projects/${param0}/episodes`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 确认完整分集边界与身份映射 POST /api/projects/${param0}/episodes/split/confirm */
export async function confirmScriptEpisodeSplit(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.confirmScriptEpisodeSplitParams,
  body: API.internalScriptAdapterHttpSplitReviewRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSplitReceipt>(
    `/api/projects/${param0}/episodes/split/confirm`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 按正文标题规则重新生成候选分集 POST /api/projects/${param0}/episodes/split/resplit */
export async function resplitScriptEpisodes(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.resplitScriptEpisodesParams,
  body: API.internalScriptAdapterHttpSplitReviewRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSplitReceipt>(
    `/api/projects/${param0}/episodes/split/resplit`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/script-file-imports */
export async function listScriptFileImports(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listScriptFileImportsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationImportPage>(
    `/api/projects/${param0}/script-file-imports`,
    {
      method: "GET",
      params: {
        // limit has a default value: 50
        limit: "50",
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/script-file-imports */
export async function createScriptFileImport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createScriptFileImportParams,
  body: API.internalScriptAdapterHttpFileImportRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/script-file-imports`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /api/projects/${param0}/script-file-imports/${param1} */
export async function getScriptFileImport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptFileImportParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, id: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationImportJob>(
    `/api/projects/${param0}/script-file-imports/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/script-file-imports/${param1}/cancel */
export async function cancelScriptFileImport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelScriptFileImportParams,
  body: API.internalScriptAdapterHttpFileImportControlRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, id: param1, ...queryParams } = params;
  return request<any>(
    `/api/projects/${param0}/script-file-imports/${param1}/cancel`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/script-file-imports/${param1}/reconcile */
export async function reconcileScriptFileImport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reconcileScriptFileImportParams,
  body: API.internalScriptAdapterHttpFileImportControlRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, id: param1, ...queryParams } = params;
  return request<any>(
    `/api/projects/${param0}/script-file-imports/${param1}/reconcile`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /api/projects/${param0}/script-file-imports/${param1}/retry */
export async function retryScriptFileImport(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.retryScriptFileImportParams,
  body: API.internalScriptAdapterHttpFileImportControlRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, id: param1, ...queryParams } = params;
  return request<any>(
    `/api/projects/${param0}/script-file-imports/${param1}/retry`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 读取指定不可变章节快照 GET /api/projects/${param0}/script-source-snapshots/${param1} */
export async function getScriptSourceSnapshot(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptSourceSnapshotParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, sid: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceSnapshotDetail>(
    `/api/projects/${param0}/script-source-snapshots/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 未完成正文保存的安全分页 GET /api/projects/${param0}/script-source-writes */
export async function listScriptSourceWrites(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listScriptSourceWritesParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceWritePage>(
    `/api/projects/${param0}/script-source-writes`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 正文保存意图状态 GET /api/projects/${param0}/script-source-writes/${param1} */
export async function getScriptSourceWrite(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptSourceWriteParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, wid: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceWriteIntent>(
    `/api/projects/${param0}/script-source-writes/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 明确取消未发布正文保存 POST /api/projects/${param0}/script-source-writes/${param1}/cancel */
export async function cancelScriptSourceWrite(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.cancelScriptSourceWriteParams,
  body: API.internalScriptAdapterHttpSourceControlRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, wid: param1, ...queryParams } = params;
  return request<any>(
    `/api/projects/${param0}/script-source-writes/${param1}/cancel`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 核验原正文保存对象与停止证据 POST /api/projects/${param0}/script-source-writes/${param1}/reconcile */
export async function reconcileScriptSourceWrite(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reconcileScriptSourceWriteParams,
  body: API.internalScriptAdapterHttpSourceControlRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, wid: param1, ...queryParams } = params;
  return request<any>(
    `/api/projects/${param0}/script-source-writes/${param1}/reconcile`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 来源章节有序分页 GET /api/projects/${param0}/script-sources */
export async function listScriptSources(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listScriptSourcesParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourcePage>(
    `/api/projects/${param0}/script-sources`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 新增剧本来源 POST /api/projects/${param0}/script-sources */
export async function createScriptSource(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createScriptSourceParams,
  body: API.internalScriptAdapterHttpSourceWriteRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceReceipt>(
    `/api/projects/${param0}/script-sources`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 来源富文本及规范原文 GET /api/projects/${param0}/script-sources/${param1} */
export async function getScriptSource(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptSourceParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, lineage: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceDetail>(
    `/api/projects/${param0}/script-sources/${param1}`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 保存来源富文本新版本 PUT /api/projects/${param0}/script-sources/${param1} */
export async function updateScriptSource(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.updateScriptSourceParams,
  body: API.internalScriptAdapterHttpSourceWriteRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, lineage: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceReceipt>(
    `/api/projects/${param0}/script-sources/${param1}`,
    {
      method: "PUT",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 从新草稿移除来源 DELETE /api/projects/${param0}/script-sources/${param1} */
export async function deleteScriptSource(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.deleteScriptSourceParams,
  body: API.internalScriptAdapterHttpSourceDeleteRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, lineage: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceReceipt>(
    `/api/projects/${param0}/script-sources/${param1}`,
    {
      method: "DELETE",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 稳定章节身份的全部快照 GET /api/projects/${param0}/script-sources/${param1}/history */
export async function listScriptSourceHistory(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listScriptSourceHistoryParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, lineage: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceHistoryPage>(
    `/api/projects/${param0}/script-sources/${param1}/history`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 原子导入完整章节批次 POST /api/projects/${param0}/script-sources/import */
export async function importScriptSources(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.importScriptSourcesParams,
  body: API.internalScriptAdapterHttpSourceImportRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceReceipt>(
    `/api/projects/${param0}/script-sources/import`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 完整来源集合重排 POST /api/projects/${param0}/script-sources/reorder */
export async function reorderScriptSources(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.reorderScriptSourcesParams,
  body: API.internalScriptAdapterHttpSourceReorderRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationSourceReceipt>(
    `/api/projects/${param0}/script-sources/reorder`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 完整剧本版本历史 GET /api/projects/${param0}/script-versions */
export async function listScriptVersions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listScriptVersionsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationVersionPage>(
    `/api/projects/${param0}/script-versions`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 采纳已确认分集的正式剧本版本 POST /api/projects/${param0}/script-versions/${param1}/adopt */
export async function adoptScriptVersion(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.adoptScriptVersionParams,
  body: API.internalScriptAdapterHttpAdoptVersionRequest,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, vid: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationAdoptReceipt>(
    `/api/projects/${param0}/script-versions/${param1}/adopt`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 候选与历史版本的规范正文片段 GET /api/projects/${param0}/script-versions/${param1}/source-text */
export async function getScriptVersionSourceText(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptVersionSourceTextParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, vid: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationVersionText>(
    `/api/projects/${param0}/script-versions/${param1}/source-text`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 分集确认的不可变历史 GET /api/projects/${param0}/script-versions/${param1}/split-confirmations */
export async function listScriptSplitConfirmations(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listScriptSplitConfirmationsParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, vid: param1, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationConfirmationPage>(
    `/api/projects/${param0}/script-versions/${param1}/split-confirmations`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 指定分集确认的完整边界与身份结果 GET /api/projects/${param0}/script-versions/${param1}/split-confirmations/${param2} */
export async function getScriptSplitConfirmation(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptSplitConfirmationParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, vid: param1, cid: param2, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptDomainSplitConfirmation>(
    `/api/projects/${param0}/script-versions/${param1}/split-confirmations/${param2}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 剧本工作区头与当前主体 GET /api/projects/${param0}/script-workspace */
export async function getScriptWorkspace(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getScriptWorkspaceParams,
  options?: import("@/lib/request").RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.githubComStephenQiu30LanverseBackendInternalScriptApplicationWorkspaceView>(
    `/api/projects/${param0}/script-workspace`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}
