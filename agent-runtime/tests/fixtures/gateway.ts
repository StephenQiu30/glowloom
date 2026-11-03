import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";
import { once } from "node:events";
import { randomUUID } from "node:crypto";
import { Pool } from "pg";
import type { AssistantMessage, Message } from "@earendil-works/pi-ai";
import { GatewayClient, type ExchangeIntent } from "../../src/model-gateway.js";
import type { RuntimeIdentity } from "../../src/protocol.js";

export const dsn = process.env.GLOWLOOM_TEST_AGENT_DB_DSN;
if (!dsn) throw new Error("isolated GLOWLOOM_TEST_AGENT_DB_DSN required");
export const pool = new Pool({ connectionString: dsn, max: 8 });
export const model = {
  release_id: "b62811dc-ef5f-49b5-823a-e159cbd95cbb",
  context_window: 8192,
  max_tokens: 512,
  input: ["text"] as ["text"],
  params: {},
};
export const identity = (): RuntimeIdentity => ({
  runtime_session_id: randomUUID(),
  run_id: randomUUID(),
  project_id: randomUUID(),
  actor_id: randomUUID(),
  owner_epoch: 1,
  control_revision: 1,
  policy_pin: "creator/v1",
  definition_pin: "coordinator/v1",
  approval_context: {
    purpose: "planning",
    candidate_id: randomUUID(),
    manifest_hash: "a".repeat(64),
  },
});
export function answer(text = "已记录目标"): AssistantMessage {
  return {
    role: "assistant",
    content: [{ type: "text", text }],
    api: "openai-completions",
    provider: "glowloom",
    model: model.release_id,
    usage: {
      input: 3,
      output: 2,
      cacheRead: 0,
      cacheWrite: 0,
      totalTokens: 5,
      cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
    },
    stopReason: "stop",
    timestamp: 0,
  };
}
export class ControlledGateway {
  readonly calls: ExchangeIntent[] = [];
  readonly receipts = new Map<string, AssistantMessage>();
  readonly requests: ExchangeIntent[] = [];
  authorized = true;
  deniedActions = new Set<string>();
  heldActions = new Set<string>();
  authorizations: string[] = [];
  heldResponses: ServerResponse[] = [];
  respond: (intent: ExchangeIntent) => AssistantMessage = () => answer();
  hold = false;
  server = createServer((req, res) => {
    void this.handle(req, res);
  });
  client!: GatewayClient;
  async start() {
    this.server.listen(0, "127.0.0.1");
    await once(this.server, "listening");
    const address = this.server.address();
    if (!address || typeof address === "string")
      throw new Error("test_server_address");
    this.client = new GatewayClient(
      `http://127.0.0.1:${address.port}`,
      "internal-test-key".repeat(3),
    );
    return this;
  }
  async close() {
    this.server.closeAllConnections();
    this.server.close();
    await once(this.server, "close");
  }
  private async handle(req: IncomingMessage, res: ServerResponse) {
    try {
      let raw = "";
      for await (const chunk of req) raw += String(chunk);
      const body = JSON.parse(raw);
      if (!this.authorized || this.deniedActions.has(body.action)) {
        res.writeHead(403).end("{}");
        return;
      }
      if (req.url === "/internal/agent/authorize") {
        this.authorizations.push(body.action);
        if (this.heldActions.has(body.action)) {
          this.heldResponses.push(res);
          return;
        }
        res.setHeader("Content-Type", "application/json");
        res.end('{"authorized":true}');
        return;
      }
      const intent = body.exchange as ExchangeIntent;
      this.requests.push(intent);
      let result = this.receipts.get(intent.exchange_id);
      if (!result) {
        this.calls.push(intent);
        result = this.respond(intent);
        this.receipts.set(intent.exchange_id, result);
      }
      if (this.hold) return; // durable gateway admitted the physical request, but Host never sees ACK
      res.setHeader("Content-Type", "application/json");
      res.end(
        JSON.stringify({
          exchange_id: intent.exchange_id,
          status: "completed",
          response: result,
        }),
      );
    } catch {
      res.writeHead(500).end("{}");
    }
  }
}
export function userText(messages: readonly Message[]): string {
  const last = [...messages].reverse().find((m) => m.role === "user");
  if (!last || last.role !== "user") return "";
  return typeof last.content === "string"
    ? last.content
    : last.content
        .filter((c) => c.type === "text")
        .map((c) => c.text)
        .join("");
}
