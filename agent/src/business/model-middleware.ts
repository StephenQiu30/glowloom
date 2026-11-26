import type { LanguageModelMiddleware } from 'ai';

export interface BudgetReservation {
  allowed: boolean;
  used: number;
  max: number;
}

export class ModelBudgetExhaustedError extends Error {
  readonly code = 'model_request_budget_exhausted';

  constructor(
    readonly used: number,
    readonly max: number,
  ) {
    super(`Model request budget exhausted (${used}/${max})`);
    this.name = 'ModelBudgetExhaustedError';
  }
}

export interface HarnessModelMiddlewareOptions {
  sessionId?: string;
  enablePromptCachePinning?: boolean;
  reserveModelRequest?: () => Promise<BudgetReservation>;
}

const TRANSIENT_TAG_RE =
  /<(?:project_digest|canvas_changes|storyboard_targets|plan)\b/i;

export function isTransientContextText(text: string): boolean {
  return TRANSIENT_TAG_RE.test(text);
}

type UnknownRecord = Record<string, unknown>;

function asRecord(value: unknown): UnknownRecord | undefined {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
    ? (value as UnknownRecord)
    : undefined;
}

function withAnthropicCacheControl(
  providerOptions: unknown,
  marker: { type: 'ephemeral' } | undefined,
): UnknownRecord | undefined {
  const root = asRecord(providerOptions)
    ? { ...asRecord(providerOptions) }
    : {};
  const anthropic = asRecord(root.anthropic)
    ? { ...asRecord(root.anthropic) }
    : {};

  if (marker) {
    anthropic.cacheControl = marker;
    root.anthropic = anthropic;
    return root;
  }

  delete anthropic.cacheControl;
  if (Object.keys(anthropic).length === 0) {
    delete root.anthropic;
  } else {
    root.anthropic = anthropic;
  }
  return Object.keys(root).length > 0 ? root : undefined;
}

function attachCacheControlToMessage(
  message: UnknownRecord,
): UnknownRecord | undefined {
  if (message.role === 'system') return undefined;
  const content = message.content;

  if (Array.isArray(content)) {
    for (let i = content.length - 1; i >= 0; i -= 1) {
      const part = asRecord(content[i]);
      if (!part) continue;
      if (
        part.type === 'text' &&
        typeof part.text === 'string' &&
        isTransientContextText(part.text)
      ) {
        continue;
      }
      const nextContent = content.slice();
      nextContent[i] = {
        ...part,
        providerOptions: withAnthropicCacheControl(part.providerOptions, {
          type: 'ephemeral',
        }),
      };
      return { ...message, content: nextContent };
    }
    return undefined;
  }

  if (typeof content === 'string' && content.length > 0) {
    if (isTransientContextText(content)) return undefined;
    return {
      ...message,
      providerOptions: withAnthropicCacheControl(message.providerOptions, {
        type: 'ephemeral',
      }),
    };
  }

  return undefined;
}

/**
 * Pure structured-prompt transformation that keeps volatile trailing context
 * blocks (canvas diff, project digest, storyboard targets, dynamic plan) out of
 * Anthropic prompt cache boundaries by anchoring `cacheControl` on the
 * immediately preceding stable block.
 */
export function pinPromptCacheAnchor<T extends UnknownRecord>(params: T): T {
  const prompt = params.prompt;
  if (!Array.isArray(prompt) || prompt.length === 0) {
    return params;
  }

  let lastIdx = prompt.length - 1;
  while (lastIdx >= 0 && asRecord(prompt[lastIdx])?.role === 'system') {
    lastIdx -= 1;
  }
  if (lastIdx < 0) return params;

  const lastMessage = asRecord(prompt[lastIdx]);
  if (!lastMessage) return params;

  const nextPrompt = prompt.slice();
  let hadTransientTail = false;

  if (Array.isArray(lastMessage.content) && lastMessage.content.length > 0) {
    const parts = lastMessage.content.slice();
    const tailIdx = parts.length - 1;
    const tailPart = asRecord(parts[tailIdx]);
    if (
      tailPart &&
      tailPart.type === 'text' &&
      typeof tailPart.text === 'string' &&
      isTransientContextText(tailPart.text)
    ) {
      hadTransientTail = true;
      const cleanedProviderOptions = withAnthropicCacheControl(
        tailPart.providerOptions,
        undefined,
      );
      const nextTail: UnknownRecord = { ...tailPart };
      if (cleanedProviderOptions) {
        nextTail.providerOptions = cleanedProviderOptions;
      } else {
        delete nextTail.providerOptions;
      }
      parts[tailIdx] = nextTail;

      let placed = false;
      for (let i = tailIdx - 1; i >= 0; i -= 1) {
        const candidate = asRecord(parts[i]);
        if (!candidate) continue;
        if (
          candidate.type === 'text' &&
          typeof candidate.text === 'string' &&
          isTransientContextText(candidate.text)
        ) {
          continue;
        }
        parts[i] = {
          ...candidate,
          providerOptions: withAnthropicCacheControl(
            candidate.providerOptions,
            { type: 'ephemeral' },
          ),
        };
        placed = true;
        break;
      }

      nextPrompt[lastIdx] = { ...lastMessage, content: parts };

      if (!placed) {
        for (let m = lastIdx - 1; m >= 0; m -= 1) {
          const prev = asRecord(nextPrompt[m]);
          if (!prev) continue;
          const anchored = attachCacheControlToMessage(prev);
          if (anchored) {
            nextPrompt[m] = anchored;
            break;
          }
        }
      }
    }
  } else if (
    typeof lastMessage.content === 'string' &&
    isTransientContextText(lastMessage.content)
  ) {
    hadTransientTail = true;
    const cleanedProviderOptions = withAnthropicCacheControl(
      lastMessage.providerOptions,
      undefined,
    );
    const nextLast: UnknownRecord = { ...lastMessage };
    if (cleanedProviderOptions) {
      nextLast.providerOptions = cleanedProviderOptions;
    } else {
      delete nextLast.providerOptions;
    }
    nextPrompt[lastIdx] = nextLast;

    for (let m = lastIdx - 1; m >= 0; m -= 1) {
      const prev = asRecord(nextPrompt[m]);
      if (!prev) continue;
      const anchored = attachCacheControlToMessage(prev);
      if (anchored) {
        nextPrompt[m] = anchored;
        break;
      }
    }
  }

  if (!hadTransientTail) {
    return params;
  }

  return {
    ...params,
    prompt: nextPrompt,
  };
}

export function createHarnessModelMiddleware(
  options: HarnessModelMiddlewareOptions,
): LanguageModelMiddleware {
  const enableCachePinning = options.enablePromptCachePinning ?? true;

  return {
    specificationVersion: 'v3',
    async transformParams({ params }) {
      let nextParams = enableCachePinning
        ? pinPromptCacheAnchor(params as unknown as UnknownRecord)
        : (params as unknown as UnknownRecord);

      if (options.sessionId) {
        const providerOptions = asRecord(nextParams.providerOptions)
          ? { ...asRecord(nextParams.providerOptions) }
          : {};
        const anthropic = asRecord(providerOptions.anthropic)
          ? { ...asRecord(providerOptions.anthropic) }
          : {};
        if (!anthropic.userId) {
          anthropic.userId = options.sessionId;
          providerOptions.anthropic = anthropic;
          nextParams = {
            ...nextParams,
            providerOptions,
          };
        }
      }

      return nextParams as typeof params;
    },
    async wrapGenerate({ doGenerate }) {
      if (options.reserveModelRequest) {
        const reservation = await options.reserveModelRequest();
        if (!reservation.allowed) {
          throw new ModelBudgetExhaustedError(
            reservation.used,
            reservation.max,
          );
        }
      }
      return doGenerate();
    },
    async wrapStream({ doStream }) {
      if (options.reserveModelRequest) {
        const reservation = await options.reserveModelRequest();
        if (!reservation.allowed) {
          throw new ModelBudgetExhaustedError(
            reservation.used,
            reservation.max,
          );
        }
      }
      return doStream();
    },
  };
}
