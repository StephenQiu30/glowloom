// @ts-ignore
/* eslint-disable */
import { request } from "@/lib/request";
import type { RequestOptions } from "@/lib/request";

/** 当前项目我的 Agent 会话 GET /api/projects/${param0}/agent/sessions */
export async function listAgentSessions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAgentSessionsParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.SessionPage>(`/api/projects/${param0}/agent/sessions`, {
    method: "GET",
    params: {
      ...queryParams,
    },
    ...(options || {}),
  });
}

/** 新建项目 Agent 私有会话 POST /api/projects/${param0}/agent/sessions */
export async function createAgentSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createAgentSessionParams,
  body: API.CreateRequest,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.SessionView>(`/api/projects/${param0}/agent/sessions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 读取 Agent 会话 GET /api/projects/${param0}/agent/sessions/${param1} */
export async function getAgentSession(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentSessionParams,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, ...queryParams } = params;
  return request<API.SessionView>(
    `/api/projects/${param0}/agent/sessions/${param1}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 分页读取 Agent 会话全部尝试摘要 GET /api/projects/${param0}/agent/sessions/${param1}/runs */
export async function listAgentRuns(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAgentRunsParams,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, ...queryParams } = params;
  return request<API.RunPage>(
    `/api/projects/${param0}/agent/sessions/${param1}/runs`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 提交 Agent 创作目标 POST /api/projects/${param0}/agent/sessions/${param1}/runs */
export async function createAgentRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.createAgentRunParams,
  body: API.StartRequest,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, ...queryParams } = params;
  return request<any>(`/api/projects/${param0}/agent/sessions/${param1}/runs`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 读取 Agent 执行快照 GET /api/projects/${param0}/agent/sessions/${param1}/runs/${param2} */
export async function getAgentRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentRunParams,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, rid: param2, ...queryParams } = params;
  return request<API.RunSnapshot>(
    `/api/projects/${param0}/agent/sessions/${param1}/runs/${param2}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 提交 Agent 控制决定 POST /api/projects/${param0}/agent/sessions/${param1}/runs/${param2}/commands */
export async function commandAgentRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.commandAgentRunParams,
  body: API.DecisionInput,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, rid: param2, ...queryParams } = params;
  return request<API.CommandReceipt>(
    `/api/projects/${param0}/agent/sessions/${param1}/runs/${param2}/commands`,
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

/** 读取 Agent 控制决定与投递回执 GET /api/projects/${param0}/agent/sessions/${param1}/runs/${param2}/commands/current */
export async function getAgentControlStatus(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentControlStatusParams,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, rid: param2, ...queryParams } = params;
  return request<API.ControlStatus>(
    `/api/projects/${param0}/agent/sessions/${param1}/runs/${param2}/commands/current`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 订阅 Agent 已提交事件 GET /api/projects/${param0}/agent/sessions/${param1}/runs/${param2}/events */
export async function subscribeAgentEvents(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.subscribeAgentEventsParams,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, rid: param2, ...queryParams } = params;
  return request<string>(
    `/api/projects/${param0}/agent/sessions/${param1}/runs/${param2}/events`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 分页读取 Agent 公开消息 GET /api/projects/${param0}/agent/sessions/${param1}/runs/${param2}/messages */
export async function listAgentMessages(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAgentMessagesParams,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, rid: param2, ...queryParams } = params;
  return request<API.MessagePage>(
    `/api/projects/${param0}/agent/sessions/${param1}/runs/${param2}/messages`,
    {
      method: "GET",
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 读取 Agent 公开消息引用原文 GET /api/projects/${param0}/agent/sessions/${param1}/runs/${param2}/messages/${param3} */
export async function getAgentMessage(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentMessageParams,
  options?: RequestOptions,
) {
  const {
    pid: param0,
    sid: param1,
    rid: param2,
    messageId: param3,
    ...queryParams
  } = params;
  return request<API.PublicMessage>(
    `/api/projects/${param0}/agent/sessions/${param1}/runs/${param2}/messages/${param3}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 找回 Agent 原输入受理回执 GET /api/projects/${param0}/agent/sessions/${param1}/submissions/${param2} */
export async function getAgentSubmission(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getAgentSubmissionParams,
  options?: RequestOptions,
) {
  const {
    pid: param0,
    sid: param1,
    requestId: param2,
    ...queryParams
  } = params;
  return request<API.AcceptedRun>(
    `/api/projects/${param0}/agent/sessions/${param1}/submissions/${param2}`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 发现 Agent 当前已受理输入 GET /api/projects/${param0}/agent/sessions/${param1}/submissions/current */
export async function getCurrentAgentSubmission(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.getCurrentAgentSubmissionParams,
  options?: RequestOptions,
) {
  const { pid: param0, sid: param1, ...queryParams } = params;
  return request<API.CurrentSubmission>(
    `/api/projects/${param0}/agent/sessions/${param1}/submissions/current`,
    {
      method: "GET",
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 列出项目创作空间内我的 Skill 版本 GET /api/projects/${param0}/agent/skills */
export async function listAgentSkills(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.listAgentSkillsParams,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.SkillVersion[]>(`/api/projects/${param0}/agent/skills`, {
    method: "GET",
    params: { ...queryParams },
    ...(options || {}),
  });
}

/** 导入私有 Skill 不可变版本 POST /api/projects/${param0}/agent/skills */
export async function installAgentSkill(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.installAgentSkillParams,
  body: API.SkillInput,
  options?: RequestOptions,
) {
  const { pid: param0, ...queryParams } = params;
  return request<API.SkillVersion>(`/api/projects/${param0}/agent/skills`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    params: { ...queryParams },
    data: body,
    ...(options || {}),
  });
}

/** 启停指定 Skill 版本 POST /api/projects/${param0}/agent/skills/${param1}/versions/${param2}/enabled */
export async function setAgentSkillEnabled(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: API.setAgentSkillEnabledParams,
  body: API.SkillEnabledRequest,
  options?: RequestOptions,
) {
  const { pid: param0, id: param1, version: param2, ...queryParams } = params;
  return request<API.SkillVersion>(
    `/api/projects/${param0}/agent/skills/${param1}/versions/${param2}/enabled`,
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
