import type { MastraModelConfig } from '@mastra/core/llm';

export interface RunIdentityHeaders {
  actorId: string;
  orgId: string;
  scopeId: string;
  projectId: string;
  runId?: string;
  traceparent?: string;
}

export interface GatewayProviderConfig {
  businessOrigin: string;
  businessToken: string;
  defaultModel: string;
}

function normalizeModelId(raw: string): `${string}/${string}` {
  if (raw.includes('/')) {
    return raw as `${string}/${string}`;
  }
  return `openai/${raw}`;
}

export function createGatewayModel(
  config: GatewayProviderConfig,
  identity: RunIdentityHeaders,
  modelKey?: string,
): MastraModelConfig {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${config.businessToken}`,
    'X-Agent-Audience': 'agent-business',
    'X-Glowloom-Actor-Id': identity.actorId,
    'X-Glowloom-Org-Id': identity.orgId,
    'X-Glowloom-Scope-Id': identity.scopeId,
    'X-Glowloom-Project-Id': identity.projectId,
  };
  if (identity.runId) {
    headers['X-Glowloom-Run-Id'] = identity.runId;
  }
  if (identity.traceparent) {
    headers.traceparent = identity.traceparent;
  }

  return {
    id: normalizeModelId(modelKey ?? config.defaultModel),
    url: `${config.businessOrigin}/internal/agent-gateway/v1`,
    apiKey: config.businessToken,
    headers,
    api: 'chat',
  };
}
