import { MCPClient } from '@mastra/mcp';
import type { RunIdentityHeaders } from './gateway-provider.js';

export interface BusinessMcpConfig {
  businessOrigin: string;
  businessToken: string;
  timeoutMs?: number;
}

export function createBusinessMcpClient(
  config: BusinessMcpConfig,
  identity: RunIdentityHeaders,
): MCPClient {
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

  return new MCPClient({
    id: `glowloom-business-mcp-${identity.runId ?? identity.projectId}`,
    timeout: config.timeoutMs ?? 15_000,
    servers: {
      glowloom: {
        url: new URL(`${config.businessOrigin}/internal/agent-mcp`),
        requestInit: { headers },
      },
    },
  });
}
