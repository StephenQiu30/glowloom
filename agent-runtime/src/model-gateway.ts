import { createHmac, createHash, randomUUID } from "node:crypto";
import {
  createAssistantMessageEventStream,
  type AssistantMessage,
  type Message,
  type Model,
  type Provider,
  type StreamOptions,
  type TranscriptContext,
} from "@earendil-works/pi-ai";
import {
  CompactionTask,
  GenerationTask,
  defineExtension,
  hook,
  type HookApi,
  type JsonObject,
} from "@earendil-works/pi-durable";
import type { Context } from "@earendil-works/chord";
import { canonicalJSON, hashJSON } from "./canonical.js";
import type { ModelPin, RuntimeIdentity } from "./protocol.js";

export type ExchangeIntent = {
  exchange_id: string;
  runtime_session_id: string;
  conversation_id: number;
  task_id: number;
  kind: "generation" | "compaction";
  model_release_id: string;
  params: Record<string, unknown>;
  messages: Message[];
  request_hash: string;
};
export interface ModelGateway {
  authorize(
    identity: RuntimeIdentity,
    action: string,
    signal?: AbortSignal,
  ): Promise<void>;
  exchange(
    identity: RuntimeIdentity,
    exchange: ExchangeIntent,
    signal?: AbortSignal,
  ): Promise<AssistantMessage>;
}

export class GatewayClient implements ModelGateway {
  private readonly origin: string;
  constructor(
    origin: string,
    private readonly signingKey: string,
  ) {
    const url = new URL(origin);
    if (
      !["http:", "https:"].includes(url.protocol) ||
      url.username ||
      url.password ||
      url.pathname !== "/" ||
      url.search ||
      url.hash ||
      signingKey.length < 32
    )
      throw new Error("invalid_gateway_configuration");
    this.origin = url.origin;
  }
  async authorize(
    identity: RuntimeIdentity,
    action: string,
    signal?: AbortSignal,
  ): Promise<void> {
    const body = await this.post(
      "/internal/agent/authorize",
      { identity, action },
      signal,
    );
    if (body.authorized !== true)
      throw new Error("runtime_authorization_denied");
  }
  async exchange(
    identity: RuntimeIdentity,
    exchange: ExchangeIntent,
    signal?: AbortSignal,
  ): Promise<AssistantMessage> {
    const body = await this.post(
      "/internal/agent/model-exchanges",
      { identity, exchange },
      signal,
    );
    if (
      body.exchange_id !== exchange.exchange_id ||
      body.status !== "completed"
    )
      throw new Error("model_exchange_needs_reconciliation");
    const result = body.response as AssistantMessage | undefined;
    if (
      !result ||
      result.role !== "assistant" ||
      !Array.isArray(result.content) ||
      !result.usage ||
      !["input", "output", "cacheRead", "cacheWrite", "totalTokens"].every(
        (k) =>
          typeof result.usage[k as keyof typeof result.usage] === "number" &&
          Number.isFinite(result.usage[k as keyof typeof result.usage]) &&
          Number(result.usage[k as keyof typeof result.usage]) >= 0,
      ) ||
      !["stop", "toolUse", "length"].includes(result.stopReason)
    )
      throw new Error("invalid_model_exchange_response");
    return result;
  }
  private async post(
    path: string,
    value: unknown,
    signal?: AbortSignal,
  ): Promise<Record<string, unknown>> {
    const body = canonicalJSON(JSON.parse(JSON.stringify(value)));
    const expires = String(Math.floor(Date.now() / 1000) + 120);
    const digest = createHash("sha256").update(body).digest("hex");
    const signature = createHmac("sha256", this.signingKey)
      .update(`POST\n${path}\n${expires}\n${digest}`)
      .digest("hex");
    const response = await fetch(`${this.origin}${path}`, {
      method: "POST",
      body,
      headers: {
        "content-type": "application/json",
        "x-glowloom-runtime-expires": expires,
        "x-glowloom-runtime-signature": signature,
      },
      signal: signal
        ? AbortSignal.any([signal, AbortSignal.timeout(90000)])
        : AbortSignal.timeout(90000),
      redirect: "error",
    });
    if (!response.ok) {
      await response.body?.cancel();
      throw new Error(
        response.status === 403 || response.status === 401
          ? "runtime_authorization_denied"
          : `runtime_gateway_${response.status}`,
      );
    }
    const chunks: Uint8Array[] = [];
    let size = 0;
    for await (const chunk of response.body ?? []) {
      size += chunk.byteLength;
      if (size > 2 * 1024 * 1024) {
        await response.body?.cancel().catch(() => {});
        throw new Error("runtime_gateway_response_too_large");
      }
      chunks.push(chunk);
    }
    return JSON.parse(Buffer.concat(chunks).toString("utf8")) as Record<
      string,
      unknown
    >;
  }
}

// One provider closure per SDK conversation. A hook failure must leave no usable request.
export function createConversationGateway(
  identity: RuntimeIdentity,
  modelPin: ModelPin,
  conversationId: number,
  gateway: ModelGateway,
  storageSignal: AbortSignal,
  recordCompactionReceipt: (receipt: JsonObject) => Promise<void>,
) {
  const providerId = `glowloom-${conversationId}`;
  let pending: ExchangeIntent | undefined;
  const pin = async (
    api: HookApi,
    messages: readonly Message[],
    kind: ExchangeIntent["kind"],
    context: Context,
  ) => {
    if (api.conversationId !== conversationId)
      throw new Error("model_conversation_mismatch");
    const fixed = JSON.parse(
      JSON.stringify({
        runtime_session_id: identity.runtime_session_id,
        conversation_id: conversationId,
        task_id: api.taskId,
        kind,
        model_release_id: modelPin.release_id,
        params: modelPin.params,
        messages,
      }),
    ) as Omit<ExchangeIntent, "exchange_id" | "request_hash">;
    const request_hash = hashJSON(fixed);
    const candidate = { ...fixed, exchange_id: randomUUID(), request_hash };
    const memo = (await api.memo(
      "glowloom.model-exchange/v1",
      candidate as unknown as JsonObject,
      context,
    )) as unknown as ExchangeIntent;
    if (
      memo.request_hash !== request_hash ||
      hashJSON({
        runtime_session_id: memo.runtime_session_id,
        conversation_id: memo.conversation_id,
        task_id: memo.task_id,
        kind: memo.kind,
        model_release_id: memo.model_release_id,
        params: memo.params,
        messages: memo.messages,
      }) !== request_hash
    )
      throw new Error("model_exchange_input_conflict");
    return memo;
  };
  const errorMessage = (message: string): AssistantMessage => ({
    role: "assistant",
    content: [],
    api: "openai-completions",
    provider: providerId,
    model: modelPin.release_id,
    usage: {
      input: 0,
      output: 0,
      cacheRead: 0,
      cacheWrite: 0,
      totalTokens: 0,
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
    },
    stopReason: "error",
    errorMessage: message,
    timestamp: Date.now(),
  });
  const stream = (
    _model: Model<"openai-completions">,
    transcript: TranscriptContext,
    options?: Pick<StreamOptions, "signal">,
  ) => {
    const result = createAssistantMessageEventStream();
    const intent = pending;
    pending = undefined;
    void (async () => {
      try {
        if (
          !intent ||
          hashJSON(intent.messages) !== hashJSON(transcript.messages)
        )
          throw new Error("model_exchange_memo_required");
        const signal = options?.signal
          ? AbortSignal.any([options.signal, storageSignal])
          : storageSignal;
        signal.throwIfAborted();
        await gateway.authorize(identity, "model.generation", signal);
        signal.throwIfAborted();
        const answer = await gateway.exchange(identity, intent, signal);
        signal.throwIfAborted();
        const message = {
          ...answer,
          provider: providerId,
          model: modelPin.release_id,
        };
        result.push({
          type: "done",
          reason: message.stopReason as "stop" | "length" | "toolUse",
          message,
        });
      } catch (error) {
        result.push({
          type: "error",
          reason: "error",
          error: errorMessage(
            error instanceof Error ? error.message : "model_exchange_failed",
          ),
        });
      }
    })();
    return result;
  };
  const provider: Provider<"openai-completions"> = {
    id: providerId,
    name: "织光模型网关",
    auth: {
      apiKey: { name: "Internal gateway", resolve: async () => ({ auth: {} }) },
    },
    getModels: () => [
      {
        id: modelPin.release_id,
        name: "Pinned dialogue model",
        api: "openai-completions",
        provider: providerId,
        baseUrl: "http://internal.invalid",
        input: modelPin.input,
        reasoning: false,
        contextWindow: modelPin.context_window,
        maxTokens: modelPin.max_tokens,
        cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      },
    ],
    stream,
    streamSimple: stream,
  };
  const extension = defineExtension({
    name: `glowloom.gateway.${conversationId}`,
    hooks: [
      hook(GenerationTask, {
        beforeRequest: async (request, api, context) => {
          pending = undefined;
          storageSignal.throwIfAborted();
          const memo = await pin(api, request.messages, "generation", context);
          await gateway.authorize(
            identity,
            "model.prepare",
            context.abortSignal
              ? AbortSignal.any([context.abortSignal, storageSignal])
              : storageSignal,
          );
          storageSignal.throwIfAborted();
          pending = memo;
          return { messages: memo.messages };
        },
      }),
      hook(CompactionTask, {
        beforeCompact: async (request, api, context) => {
          const signal = context.abortSignal
            ? AbortSignal.any([context.abortSignal, storageSignal])
            : storageSignal;
          signal.throwIfAborted();
          const messages: Message[] = [
            {
              role: "system",
              content:
                "总结以下创作记录，保留已采用事实、来源、权限与未完成事项。不得增加批准或声明未发生的执行。",
              timestamp: 0,
            },
            {
              role: "user",
              content: canonicalJSON({
                messages: request.messages,
                instructions: request.instructions ?? "",
              }),
              timestamp: 0,
            },
          ];
          const memo = await pin(api, messages, "compaction", context);
          await gateway.authorize(identity, "model.compaction", signal);
          signal.throwIfAborted();
          const result = await gateway.exchange(identity, memo, signal);
          signal.throwIfAborted();
          await recordCompactionReceipt(
            JSON.parse(
              JSON.stringify({
                conversation_id: api.conversationId,
                task_id: api.taskId,
                exchange_id: memo.exchange_id,
                usage: result.usage,
              }),
            ) as JsonObject,
          );
          if (
            result.stopReason !== "stop" ||
            result.content.some((block) => block.type === "toolCall")
          )
            throw new Error("invalid_compaction_response");
          const summary = result.content
            .filter((c) => c.type === "text")
            .map((c) => c.text)
            .join("\n")
            .trim();
          if (!summary) throw new Error("empty_compaction_summary");
          return { summary };
        },
      }),
    ],
  });
  return { provider, extension };
}
