// @ts-ignore
/* eslint-disable */
import { request } from '../request.js';
import type { RequestOptions } from '../request.js';

/** 此处后端没有提供注释 POST /internal/agent-business/grants */
export async function registerGrant(
  body: AgentBusinessApi.GrantRequest,
  options?: RequestOptions,
) {
  return request<AgentBusinessApi.Grant>('/internal/agent-business/grants', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 POST /internal/agent-business/grants/validate */
export async function validateGrant(
  body: AgentBusinessApi.Grant,
  options?: RequestOptions,
) {
  return request<AgentBusinessApi.Grant>(
    '/internal/agent-business/grants/validate',
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /internal/agent-business/input-sources/validate */
export async function validateInputSources(
  body: AgentBusinessApi.businessInputSources,
  options?: RequestOptions,
) {
  return request<AgentBusinessApi.businessValid>(
    '/internal/agent-business/input-sources/validate',
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /internal/agent-business/models/stream */
export async function executeModel(
  body: AgentBusinessApi.CallInput,
  options?: RequestOptions,
) {
  return request<AgentBusinessApi.CallReceipt>(
    '/internal/agent-business/models/stream',
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /internal/agent-business/proposals */
export async function createProposal(
  body: AgentBusinessApi.ProposalInput,
  options?: RequestOptions,
) {
  return request<AgentBusinessApi.businessProposal>(
    '/internal/agent-business/proposals',
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /internal/agent-business/runs/${param0}/calls/${param1} */
export async function getCall(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: AgentBusinessApi.getCallParams,
  options?: RequestOptions,
) {
  const { rid: param0, call_id: param1, ...queryParams } = params;
  return request<AgentBusinessApi.CallReceipt>(
    `/internal/agent-business/runs/${param0}/calls/${param1}`,
    {
      method: 'GET',
      params: {
        ...queryParams,
      },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 GET /internal/agent-business/runs/${param0}/decisions */
export async function getDecisions(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: AgentBusinessApi.getDecisionsParams,
  options?: RequestOptions,
) {
  const { rid: param0, ...queryParams } = params;
  return request<AgentBusinessApi.Authority>(
    `/internal/agent-business/runs/${param0}/decisions`,
    {
      method: 'GET',
      params: { ...queryParams },
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /internal/agent-business/runs/${param0}/quiesced */
export async function quiesceRun(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: AgentBusinessApi.quiesceRunParams,
  body: AgentBusinessApi.QuiescedReceipt,
  options?: RequestOptions,
) {
  const { rid: param0, ...queryParams } = params;
  return request<AgentBusinessApi.Grant>(
    `/internal/agent-business/runs/${param0}/quiesced`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /internal/agent-business/runs/${param0}/receipts */
export async function acknowledgeReceipt(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: AgentBusinessApi.acknowledgeReceiptParams,
  body: AgentBusinessApi.AppliedReceipt,
  options?: RequestOptions,
) {
  const { rid: param0, ...queryParams } = params;
  return request<AgentBusinessApi.businessAccepted>(
    `/internal/agent-business/runs/${param0}/receipts`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /internal/agent-business/skills/${param0}/load */
export async function loadSkill(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: AgentBusinessApi.loadSkillParams,
  body: AgentBusinessApi.businessSkillLoad,
  options?: RequestOptions,
) {
  const { id: param0, ...queryParams } = params;
  return request<AgentBusinessApi.SkillContent>(
    `/internal/agent-business/skills/${param0}/load`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /internal/agent-business/skills/discover */
export async function discoverSkills(
  body: AgentBusinessApi.businessSkillDiscover,
  options?: RequestOptions,
) {
  return request<AgentBusinessApi.SkillVersion[]>(
    '/internal/agent-business/skills/discover',
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      data: body,
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /internal/agent-business/snapshot */
export async function getSnapshot(
  body: AgentBusinessApi.Grant,
  options?: RequestOptions,
) {
  return request<Record<string, any>>('/internal/agent-business/snapshot', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    data: body,
    ...(options || {}),
  });
}

/** 此处后端没有提供注释 GET /internal/agent-business/tools */
export async function listTools(options?: RequestOptions) {
  return request<AgentBusinessApi.ToolDescriptor[]>(
    '/internal/agent-business/tools',
    {
      method: 'GET',
      ...(options || {}),
    },
  );
}

/** 此处后端没有提供注释 POST /internal/agent-business/tools/${param0}/execute */
export async function executeTool(
  // 叠加生成的Param类型 (非body参数swagger默认没有生成对象)
  params: AgentBusinessApi.executeToolParams,
  body: AgentBusinessApi.CallInput,
  options?: RequestOptions,
) {
  const { name: param0, ...queryParams } = params;
  return request<AgentBusinessApi.CallReceipt>(
    `/internal/agent-business/tools/${param0}/execute`,
    {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      params: { ...queryParams },
      data: body,
      ...(options || {}),
    },
  );
}
