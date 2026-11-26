import { createAnthropic } from '@ai-sdk/anthropic';
import { createGoogleGenerativeAI } from '@ai-sdk/google';
import { createOpenAI } from '@ai-sdk/openai';
import { createOpenAICompatible } from '@ai-sdk/openai-compatible';
import type { MastraModelConfig } from '@mastra/core/llm';
import { wrapLanguageModel } from 'ai';
import {
  createHarnessModelMiddleware,
  type BudgetReservation,
} from './model-middleware.js';

export type GatewayProtocol =
  'openai-compatible' | 'openai' | 'openai-responses' | 'anthropic' | 'google';

export interface RunIdentityHeaders {
  actorId: string;
  orgId: string;
  scopeId: string;
  projectId: string;
  sessionId?: string;
  runId?: string;
  traceparent?: string;
}

export interface GatewayProviderConfig {
  businessOrigin: string;
  businessToken: string;
  defaultModel: string;
  protocol?: GatewayProtocol;
}

export interface CreateGatewayModelOptions {
  modelKey?: string;
  protocol?: GatewayProtocol;
  reserveModelRequest?: () => Promise<BudgetReservation>;
}

function buildGatewayHeaders(
  businessToken: string,
  identity: RunIdentityHeaders,
): Record<string, string> {
  const headers: Record<string, string> = {
    Authorization: `Bearer ${businessToken}`,
    'X-Agent-Audience': 'agent-business',
    'X-Glowloom-Actor-Id': identity.actorId,
    'X-Glowloom-Org-Id': identity.orgId,
    'X-Glowloom-Scope-Id': identity.scopeId,
    'X-Glowloom-Project-Id': identity.projectId,
  };
  if (identity.sessionId) {
    headers['X-Glowloom-Session-Id'] = identity.sessionId;
  }
  if (identity.runId) {
    headers['X-Glowloom-Run-Id'] = identity.runId;
  }
  if (identity.traceparent) {
    headers.traceparent = identity.traceparent;
  }
  return headers;
}

/**
 * Instantiates a native Vercel AI SDK provider pointed at the Go unified
 * channel gateway without any hand-written `fetch` or HTTP proxy code, then
 * wraps it with the Harness LanguageModelMiddleware (Prompt Cache stable anchor
 * pinning, Anthropic session affinity, and pre-dispatch durable budget checks).
 */
export function createGatewayModel(
  config: GatewayProviderConfig,
  identity: RunIdentityHeaders,
  options: CreateGatewayModelOptions | string = {},
): MastraModelConfig {
  const normalized: CreateGatewayModelOptions =
    typeof options === 'string' ? { modelKey: options } : options;
  const modelId = normalized.modelKey ?? config.defaultModel;
  const protocol: GatewayProtocol =
    normalized.protocol ?? config.protocol ?? 'openai-compatible';
  const headers = buildGatewayHeaders(config.businessToken, identity);
  const gatewayRoot = `${config.businessOrigin}/internal/agent-gateway`;

  let baseModel;
  switch (protocol) {
    case 'openai':
      baseModel = createOpenAI({
        baseURL: `${gatewayRoot}/openai/v1`,
        apiKey: config.businessToken,
        headers,
      }).chat(modelId);
      break;
    case 'openai-responses':
      baseModel = createOpenAI({
        baseURL: `${gatewayRoot}/openai/v1`,
        apiKey: config.businessToken,
        headers,
      }).responses(modelId);
      break;
    case 'anthropic':
      baseModel = createAnthropic({
        baseURL: `${gatewayRoot}/anthropic/v1`,
        apiKey: config.businessToken,
        headers,
      })(modelId);
      break;
    case 'google':
      baseModel = createGoogleGenerativeAI({
        baseURL: `${gatewayRoot}/google/v1beta`,
        apiKey: config.businessToken,
        headers,
      })(modelId);
      break;
    case 'openai-compatible':
    default:
      baseModel = createOpenAICompatible({
        name: 'glowloom',
        baseURL: `${gatewayRoot}/v1`,
        apiKey: config.businessToken,
        headers,
      }).chatModel(modelId);
      break;
  }

  return wrapLanguageModel({
    model: baseModel,
    middleware: createHarnessModelMiddleware({
      ...(identity.sessionId ? { sessionId: identity.sessionId } : {}),
      ...(normalized.reserveModelRequest
        ? { reserveModelRequest: normalized.reserveModelRequest }
        : {}),
    }),
  }) as unknown as MastraModelConfig;
}
