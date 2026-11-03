import { BACKGROUND_CONTEXT as context } from "@earendil-works/chord/context";
import { createModels } from "@earendil-works/pi-ai/models";
import {
  Harness,
  ROOT_CONVERSATION_ID,
  defineDoc,
  defineDocFamily,
  type Conversation,
  type ConversationId,
  type JsonObject,
  type SubmissionId,
  type TaskId,
  type CompactionResult,
} from "@earendil-works/pi-durable";
import type { Pool } from "pg";
import { hashJSON } from "./canonical.js";
import {
  createConversationGateway,
  type ModelGateway,
} from "./model-gateway.js";
import { openPostgresStorage } from "./postgres-storage.js";
import {
  SDK_VERSION,
  isUUID,
  validateIdentity,
  validateModel,
  type CommandReceipt,
  type InputCommand,
  type ModelPin,
  type RuntimeIdentity,
} from "./protocol.js";

const HostDoc = defineDoc<{
  binding: string;
  model_hash: string;
  max_epoch: number;
  control_revision: number;
  phase_hash: string;
}>({
  kind: "glowloom.host",
  version: 1,
  scope: "session",
  initial: () => ({
    binding: "",
    model_hash: "",
    max_epoch: 0,
    control_revision: 0,
    phase_hash: "",
  }),
});
const CommandDoc = defineDocFamily<
  { payload_hash: string; receipt: CommandReceipt | null },
  null
>({
  kind: "glowloom.command",
  version: 1,
  scope: "session",
  family: true,
  initial: () => ({ payload_hash: "", receipt: null }),
});

const CompactionReceiptDoc = defineDocFamily<
  { hash: string; receipt: JsonObject | null },
  null
>({
  kind: "glowloom.compaction-receipt",
  version: 1,
  scope: "conversation",
  history: "latest",
  fork: "initial",
  family: true,
  initial: () => ({ hash: "", receipt: null }),
});

type OpenOptions = {
  pool: Pool;
  identity: RuntimeIdentity;
  model: ModelPin;
  gateway: ModelGateway;
};

export async function openOwnedSession(
  options: OpenOptions,
): Promise<OwnedSession> {
  const { pool, gateway } = options;
  const identity = structuredClone(options.identity);
  const model = structuredClone(options.model);
  validateIdentity(identity);
  validateModel(model);
  await gateway.authorize(identity, "session.open");
  const storage = await openPostgresStorage(pool, identity.runtime_session_id);
  let harness: Harness | undefined;
  try {
    const models = createModels({
      authContext: {
        env: async () => undefined,
        fileExists: async () => false,
      },
    });
    const sdk = await import("@earendil-works/pi-durable");
    const registry = sdk.createRegistry();
    const bridge = createConversationGateway(
      identity,
      model,
      ROOT_CONVERSATION_ID,
      gateway,
      storage.signal,
      async (receipt) => {
        if (!harness) throw new Error("runtime_not_open");
        storage.signal.throwIfAborted();
        await harness.commit(async (tx) => {
          const doc = await tx.doc(
            CompactionReceiptDoc,
            Number(receipt.conversation_id) as ConversationId,
            String(receipt.task_id),
            null,
          );
          const hash = hashJSON(receipt);
          if (doc.hash && doc.hash !== hash)
            throw new Error("compaction_receipt_conflict");
          doc.hash = hash;
          doc.receipt = receipt;
        }, context);
      },
    );
    models.setProvider(bridge.provider);
    registry.install(bridge.extension);
    harness = await Harness.open(
      storage,
      {
        models,
        registry,
        settings: {
          retry: { enabled: false, maxRetries: 0 },
          stream: { maxRetries: 0 },
          compaction: {
            enabled: true,
            reserveTokens: model.max_tokens,
            keepRecentTokens: 128,
            backgroundTokens: 0,
          },
        },
      },
      context,
    );
    const activeHarness = harness;
    const closeOnStorageFailure = () => {
      void activeHarness.close(context).catch(() => {});
    };
    storage.signal.addEventListener("abort", closeOnStorageFailure, {
      once: true,
    });
    void activeHarness.closed.then(() =>
      storage.signal.removeEventListener("abort", closeOnStorageFailure),
    );
    storage.signal.throwIfAborted();
    const binding = hashJSON({
      runtime_session_id: identity.runtime_session_id,
      project_id: identity.project_id,
      actor_id: identity.actor_id,
      run_id: identity.run_id,
      policy_pin: identity.policy_pin,
      definition_pin: identity.definition_pin,
    });
    const phaseHash = hashJSON(identity.approval_context);
    await harness.commit(async (tx) => {
      const doc = await tx.doc(HostDoc);
      if (
        doc.binding &&
        (doc.binding !== binding || doc.model_hash !== hashJSON(model))
      )
        throw new Error("runtime_identity_conflict");
      if (identity.owner_epoch < doc.max_epoch)
        throw new Error("runtime_owner_fenced");
      if (
        identity.control_revision < (doc.control_revision ?? 0) ||
        (identity.control_revision === (doc.control_revision ?? 0) &&
          doc.phase_hash &&
          doc.phase_hash !== phaseHash)
      )
        throw new Error("runtime_owner_fenced");
      doc.binding = binding;
      doc.model_hash = hashJSON(model);
      doc.max_epoch = identity.owner_epoch;
      doc.control_revision = identity.control_revision;
      doc.phase_hash = phaseHash;
    }, context);
    const root = await harness.root(context, {
      agent: {
        model: { provider: bridge.provider.id, modelId: model.release_id },
        extensions: [bridge.extension],
        tools: [],
        instructions:
          "你是织光创作协调助手。仅依据授权输入与正式回执形成创作计划，不自行签发批准或宣称未验证的产物已完成。",
      },
    });
    harness.resume();
    return new OwnedSession(harness, root, identity, gateway);
  } catch (error) {
    if (harness) await harness.close(context);
    else await storage.close(context);
    throw error;
  }
}

export class OwnedSession {
  private line: Promise<unknown> = Promise.resolve();
  private closing: Promise<void> | undefined;
  readonly closed: Promise<void>;
  constructor(
    private readonly harness: Harness,
    private readonly root: Conversation,
    private readonly identity: RuntimeIdentity,
    private readonly gateway: ModelGateway,
  ) {
    this.closed = harness.closed.then(() => {});
  }

  async acceptCommand(input: InputCommand): Promise<CommandReceipt> {
    if (this.closing) throw new Error("runtime_session_draining");
    // Serialize admission and receipt publication, independent of concurrent HTTP dispatchers.
    const next = this.line.then(() => this.accept(input));
    this.line = next.catch(() => {});
    return next;
  }
  private async accept(input: InputCommand): Promise<CommandReceipt> {
    if (
      !isUUID(input.request_id) ||
      typeof input.message !== "string" ||
      !input.message.trim() ||
      Buffer.byteLength(input.message) > 256 * 1024
    )
      throw new Error("invalid_runtime_command");
    await this.gateway.authorize(this.identity, "command.submit");
    const hash = hashJSON(input);
    const receipt = await this.harness.commit(async (tx) => {
      const doc = await tx.doc(CommandDoc, input.request_id, null);
      if (doc.payload_hash && doc.payload_hash !== hash)
        throw new Error("runtime_command_conflict");
      doc.payload_hash = hash;
      return doc.receipt ? { ...doc.receipt } : null;
    }, context);
    if (receipt) {
      this.harness.resume();
      return receipt;
    }
    const submission = await this.root.submit(
      {
        type: "input",
        content: input.message,
        requestId: input.request_id,
        whenBusy: "followUp",
      },
      context,
    );
    const accepted = {
      request_id: input.request_id,
      submission_id: submission.id,
      root_conversation_id: this.root.id,
    };
    await this.harness.commit(async (tx) => {
      (await tx.doc(CommandDoc, input.request_id, null)).receipt = accepted;
    }, context);
    return accepted;
  }
  async waitSubmission(id: number) {
    const submission = await this.harness.submission(
      id as SubmissionId,
      context,
    );
    if (!submission) throw new Error("runtime_submission_not_found");
    return submission.wait(context);
  }
  async compact(instructions: string) {
    await this.gateway.authorize(this.identity, "command.compact");
    const id = await this.root.compact(instructions, context);
    const result = await this.harness.waitForTask(
      id as TaskId<CompactionResult>,
      context,
    );
    if (result.state.outcome.status !== "completed")
      throw new Error("runtime_compaction_failed");
    return result.state.outcome.result;
  }
  async snapshot() {
    await this.gateway.authorize(this.identity, "session.snapshot");
    const inspection = await this.harness.inspect(context);
    const history = await this.root.entries({}, 100, undefined, context);
    // Never project private model thinking or arbitrary task input/checkpoints.
    const entries = history.items.map((entry) => ({
      id: entry.id,
      kind: entry.kind,
      model: entry.model?.map((message) =>
        message.role === "assistant"
          ? {
              ...message,
              content: message.content.filter(
                (block) => block.type !== "thinking",
              ),
            }
          : message,
      ),
    }));
    return {
      runtime_session_id: this.identity.runtime_session_id,
      sdk_version: SDK_VERSION,
      root_conversation_id: this.root.id,
      scheduling: inspection.scheduling,
      entries,
      has_more: history.next !== undefined,
      tasks: inspection.tasks.map((task) => ({
        id: task.record.id,
        kind: task.record.kind,
        state: task.record.state.status,
      })),
      submissions: inspection.submissions.map((s) => ({
        id: s.id,
        status: s.status,
      })),
    };
  }
  close(): Promise<void> {
    this.closing ??= this.line.then(() => this.harness.close(context));
    return this.closing;
  }
}
