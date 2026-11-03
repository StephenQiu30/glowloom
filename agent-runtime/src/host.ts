import { createHash, createHmac, timingSafeEqual } from "node:crypto";
import {
  createServer,
  type IncomingMessage,
  type ServerResponse,
} from "node:http";
import { pathToFileURL } from "node:url";
import { Pool } from "pg";
import { hashJSON } from "./canonical.js";
import { GatewayClient, type ModelGateway } from "./model-gateway.js";
import {
  PROTOCOL,
  SDK_VERSION,
  isUUID,
  validateIdentity,
  validateModel,
  type InputCommand,
  type ModelPin,
  type RuntimeIdentity,
} from "./protocol.js";
import { openOwnedSession, type OwnedSession } from "./session-owner.js";

type HostOptions = {
  pool: Pool;
  gateway: ModelGateway;
  signingKey: string;
  maxSessions?: number;
};
type Envelope = {
  protocol: string;
  identity: RuntimeIdentity;
  model: ModelPin;
  command: InputCommand;
  payload_hash: string;
};

function authenticated(
  req: IncomingMessage,
  body: Buffer,
  key: string,
): boolean {
  const expires = req.headers["x-glowloom-runtime-expires"];
  const signature = req.headers["x-glowloom-runtime-signature"];
  if (
    typeof expires !== "string" ||
    !/^\d{10}$/.test(expires) ||
    typeof signature !== "string" ||
    !/^[a-f0-9]{64}$/.test(signature)
  )
    return false;
  const remaining = Number(expires) - Math.floor(Date.now() / 1000);
  if (remaining <= 0 || remaining > 120) return false;
  const hash = createHash("sha256").update(body).digest("hex");
  const expected = createHmac("sha256", key)
    .update(`${req.method}\n${req.url}\n${expires}\n${hash}`)
    .digest();
  return timingSafeEqual(expected, Buffer.from(signature, "hex"));
}
async function readBody(req: IncomingMessage): Promise<Buffer> {
  const chunks: Buffer[] = [];
  let size = 0;
  for await (const chunk of req) {
    size += chunk.length;
    if (size > 512 * 1024) throw new Error("runtime_body_too_large");
    chunks.push(chunk);
  }
  return Buffer.concat(chunks);
}
function respond(res: ServerResponse, status: number, body: unknown) {
  res.writeHead(status, {
    "content-type": "application/json",
    "cache-control": "no-store",
  });
  res.end(JSON.stringify(body));
}
function statusFor(error: unknown): [number, string] {
  const code = error instanceof Error ? error.message : "runtime_unavailable";
  if (code === "runtime_authorization_denied") return [403, code];
  if (code === "runtime_unauthenticated") return [401, code];
  if (code === "runtime_capacity") return [429, code];
  if (code === "runtime_body_too_large") return [413, code];
  if (
    [
      "runtime_command_conflict",
      "runtime_identity_conflict",
      "runtime_session_owned",
      "runtime_owner_fenced",
    ].includes(code)
  )
    return [409, code];
  if (
    code.startsWith("invalid_") ||
    error instanceof SyntaxError ||
    error instanceof TypeError
  )
    return [400, "invalid_runtime_request"];
  // Driver messages can contain connection details; never project arbitrary exceptions.
  return [503, "runtime_unavailable"];
}

export function createRuntimeServer(options: HostOptions) {
  if (options.signingKey.length < 32)
    throw new Error("invalid_runtime_signing_key");
  const capacity = options.maxSessions ?? 16;
  if (!Number.isSafeInteger(capacity) || capacity < 1)
    throw new Error("invalid_runtime_capacity");
  const sessions = new Map<
    string,
    {
      binding: string;
      identity: RuntimeIdentity;
      session: Promise<OwnedSession>;
    }
  >();
  let closing = false;
  const server = createServer((req, res) => {
    void handle(req, res);
  });
  server.requestTimeout = 15000;
  server.headersTimeout = 10000;
  const handle = async (req: IncomingMessage, res: ServerResponse) => {
    try {
      if (req.method === "GET" && req.url === "/healthz") {
        respond(res, 200, {
          status: "ok",
          protocol: PROTOCOL,
          sdk_version: SDK_VERSION,
          storage: "postgresql",
        });
        return;
      }
      if (closing) throw new Error("runtime_closing");
      if (req.method === "GET" && req.url === "/readyz") {
        await options.pool.query("SELECT 1");
        respond(res, 200, {
          status: "ok",
          checks: ["process", "postgresql"],
          protocol: PROTOCOL,
        });
        return;
      }
      const body = await readBody(req);
      if (closing) throw new Error("runtime_closing");
      if (!authenticated(req, body, options.signingKey))
        throw new Error("runtime_unauthenticated");
      if (req.method === "POST" && req.url === "/internal/v1/commands") {
        const envelope = JSON.parse(body.toString("utf8")) as Envelope;
        if (envelope.protocol !== PROTOCOL)
          throw new Error("invalid_runtime_protocol");
        validateIdentity(envelope.identity);
        validateModel(envelope.model);
        const { identity, model, command } = envelope;
        if (
          !command ||
          hashJSON({ identity, model, command }) !== envelope.payload_hash
        )
          throw new Error("invalid_runtime_payload_hash");
        const sid = identity.runtime_session_id;
        const binding = hashJSON({ identity, model });
        const current = sessions.get(sid);
        if (current && current.binding !== binding)
          throw new Error("runtime_identity_conflict");
        if (!current) {
          if (sessions.size >= capacity) throw new Error("runtime_capacity");
          const session = openOwnedSession({
            pool: options.pool,
            identity,
            model,
            gateway: options.gateway,
          });
          const entry = {
            binding,
            identity: structuredClone(identity),
            session,
          };
          sessions.set(sid, entry);
          const remove = () => {
            if (sessions.get(sid) === entry) sessions.delete(sid);
          };
          void session.then((owner) => owner.closed.then(remove), remove);
        }
        const session = await sessions.get(sid)!.session;
        if (closing) throw new Error("runtime_closing");
        respond(res, 202, await session.acceptCommand(command));
        return;
      }
      const snapshot = req.url?.match(
        /^\/internal\/v1\/sessions\/([^/]+)\/snapshot$/,
      );
      if (req.method === "GET" && snapshot && isUUID(snapshot[1])) {
        const session = sessions.get(snapshot[1]);
        if (!session) {
          respond(res, 404, { code: "runtime_session_not_open" });
          return;
        }
        respond(res, 200, await (await session.session).snapshot());
        return;
      }
      const drain = req.url?.match(/^\/internal\/v1\/owners\/([^/]+)\/drain$/);
      if (req.method === "POST" && drain && isUUID(drain[1])) {
        const envelope = JSON.parse(body.toString("utf8")) as {
          protocol: string;
          identity: RuntimeIdentity;
          payload_hash: string;
        };
        validateIdentity(envelope.identity);
        if (
          envelope.protocol !== PROTOCOL ||
          envelope.identity.runtime_session_id !== drain[1] ||
          envelope.payload_hash !== hashJSON({ identity: envelope.identity })
        )
          throw new Error("invalid_runtime_drain");
        const session = sessions.get(drain[1]);
        if (
          session &&
          (session.identity.runtime_session_id !==
            envelope.identity.runtime_session_id ||
            session.identity.run_id !== envelope.identity.run_id ||
            session.identity.project_id !== envelope.identity.project_id ||
            session.identity.actor_id !== envelope.identity.actor_id ||
            session.identity.owner_epoch !== envelope.identity.owner_epoch ||
            session.identity.policy_pin !== envelope.identity.policy_pin ||
            session.identity.definition_pin !==
              envelope.identity.definition_pin ||
            session.identity.control_revision >
              envelope.identity.control_revision)
        )
          throw new Error("runtime_owner_fenced");
        await options.gateway.authorize(envelope.identity, "owner.drain");
        if (session && sessions.get(drain[1]) !== session)
          throw new Error("runtime_owner_fenced");
        if (session) {
          await (await session.session).close();
          if (sessions.get(drain[1]) === session) sessions.delete(drain[1]);
        }
        respond(res, 200, { status: "drained" });
        return;
      }
      respond(res, 404, { code: "runtime_route_not_found" });
    } catch (error) {
      const [status, code] = statusFor(error);
      respond(res, status, { code });
    }
  };
  return {
    server,
    close: async () => {
      closing = true;
      const stopped = server.listening
        ? new Promise<void>((resolve, reject) => {
            server.close((error) => (error ? reject(error) : resolve()));
          })
        : Promise.resolve();
      const results = await Promise.allSettled(
        [...sessions.values()].map(async (item) => {
          await (await item.session).close();
        }),
      );
      sessions.clear();
      server.closeAllConnections();
      await stopped;
      const failed = results.find((result) => result.status === "rejected");
      if (failed?.status === "rejected") throw failed.reason;
    },
  };
}

async function main() {
  const dsn = process.env.GLOWLOOM_AGENT_DATABASE_DSN;
  const origin = process.env.GLOWLOOM_AGENT_GATEWAY_ORIGIN;
  const signingKey = process.env.GLOWLOOM_AGENT_SIGNING_KEY;
  if (!dsn || !origin || !signingKey)
    throw new Error(
      "agent runtime requires explicit database, gateway and signing configuration",
    );
  const pool = new Pool({
    connectionString: dsn,
    max: 20,
    connectionTimeoutMillis: 5000,
    query_timeout: 15000,
    statement_timeout: 10000,
  });
  const host = createRuntimeServer({
    pool,
    gateway: new GatewayClient(origin, signingKey),
    signingKey,
  });
  const port = Number(process.env.GLOWLOOM_AGENT_PORT ?? 4401);
  if (!Number.isInteger(port) || port < 1 || port > 65535)
    throw new Error("invalid_runtime_port");
  host.server.listen(port, "0.0.0.0");
  const shutdown = () => {
    const deadline = setTimeout(() => process.exit(1), 15000);
    deadline.unref();
    void host
      .close()
      .then(() => pool.end())
      .then(
        () => {
          clearTimeout(deadline);
        },
        () => {
          process.exitCode = 1;
        },
      );
  };
  process.once("SIGTERM", shutdown);
  process.once("SIGINT", shutdown);
}
if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  void main().catch(() => {
    console.error("agent runtime startup failed");
    process.exitCode = 1;
  });
}
