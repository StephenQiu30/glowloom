import path from "node:path";
import { fileURLToPath } from "node:url";
import * as grpc from "@grpc/grpc-js";
import * as protoLoader from "@grpc/proto-loader";
import { computeCandidate } from "../runtime/candidate.js";
import { buildExpectedBearer, verifyBearerToken } from "./auth.js";
import { classifyRuntimeError, ConcurrencyGate } from "./concurrency.js";

const maxMessageBytes = 16 * 1024 * 1024;

interface ComputeRequest {
  payload_json?: Buffer;
  update_bytes?: Buffer;
}

interface ComputeResponse {
  payload_json: Buffer;
  error_code: string;
  definitive: boolean;
}

interface HealthResponse {
  status: string;
}

export interface CanvasGrpcServer {
  address: string;
  close(): Promise<void>;
}

function resolveProtoPath(): string {
  const currentDir = path.dirname(fileURLToPath(import.meta.url));
  return path.resolve(currentDir, "../../proto/runtime.proto");
}

function verifyToken(
  metadata: grpc.Metadata,
  expected: Buffer,
): Partial<grpc.StatusObject> | null {
  const values = metadata.get("authorization");
  const raw = typeof values[0] === "string" ? values[0] : undefined;
  if (!verifyBearerToken(raw, expected)) {
    return {
      code: grpc.status.UNAUTHENTICATED,
      details: "UNAUTHORIZED",
    };
  }
  return null;
}

export async function startGrpcServer({
  address,
  token,
  maxConcurrent = 4,
  gate,
}: {
  address: string;
  token: string | undefined;
  maxConcurrent?: number;
  gate?: ConcurrencyGate;
}): Promise<CanvasGrpcServer> {
  const expected = buildExpectedBearer(token);
  const limiter = gate ?? new ConcurrencyGate(maxConcurrent);
  const packageDef = protoLoader.loadSync(resolveProtoPath(), {
    keepCase: true,
    longs: String,
    enums: String,
    defaults: true,
    oneofs: true,
  });
  const loaded = grpc.loadPackageDefinition(packageDef) as unknown as {
    glowloom: {
      canvas: {
        CanvasRuntime: { service: grpc.ServiceDefinition };
      };
    };
  };

  const makeHandler =
    (action: "initialize" | "prepare" | "rename" | "commands") =>
    async (
      call: grpc.ServerUnaryCall<ComputeRequest, ComputeResponse>,
      callback: grpc.sendUnaryData<ComputeResponse>,
    ) => {
      const authErr = verifyToken(call.metadata, expected);
      if (authErr) {
        callback(authErr);
        return;
      }
      const release = limiter.tryAcquire();
      if (!release) {
        callback(
          Object.assign(new Error("RUNTIME_BUSY"), {
            code: grpc.status.RESOURCE_EXHAUSTED,
          }),
        );
        return;
      }
      try {
        const rawJson = call.request.payload_json ?? Buffer.alloc(0);
        if (rawJson.length > maxMessageBytes) {
          callback(
            Object.assign(new Error("REQUEST_TOO_LARGE"), {
              code: grpc.status.INVALID_ARGUMENT,
            }),
          );
          return;
        }
        let input: Record<string, unknown>;
        try {
          input = JSON.parse(rawJson.toString("utf8")) as Record<
            string,
            unknown
          >;
        } catch {
          callback(
            Object.assign(new Error("INVALID_JSON"), {
              code: grpc.status.INVALID_ARGUMENT,
            }),
          );
          return;
        }
        if (
          action === "prepare" &&
          call.request.update_bytes &&
          call.request.update_bytes.length > 0
        ) {
          input.update = Buffer.from(call.request.update_bytes).toString(
            "base64",
          );
        }
        const result = await computeCandidate(action, input);
        callback(null, {
          payload_json: Buffer.from(JSON.stringify(result), "utf8"),
          error_code: "",
          definitive: false,
        });
      } catch (error) {
        const { code, definitive } = classifyRuntimeError(error);
        if (definitive) {
          callback(null, {
            payload_json: Buffer.alloc(0),
            error_code: code,
            definitive: true,
          });
          return;
        }
        callback(
          Object.assign(new Error(code), { code: grpc.status.UNAVAILABLE }),
        );
      } finally {
        release();
      }
    };

  const server = new grpc.Server({
    "grpc.max_receive_message_length": maxMessageBytes,
    "grpc.max_send_message_length": maxMessageBytes,
  });

  server.addService(loaded.glowloom.canvas.CanvasRuntime.service, {
    Initialize: makeHandler("initialize"),
    Prepare: makeHandler("prepare"),
    Rename: makeHandler("rename"),
    Commands: makeHandler("commands"),
    CheckHealth: (
      _call: grpc.ServerUnaryCall<Record<string, never>, HealthResponse>,
      callback: grpc.sendUnaryData<HealthResponse>,
    ) => {
      callback(null, { status: "ok" });
    },
  });

  const boundPort = await new Promise<number>((resolve, reject) => {
    server.bindAsync(
      address,
      grpc.ServerCredentials.createInsecure(),
      (err, port) => {
        if (err) {
          reject(err);
          return;
        }
        resolve(port);
      },
    );
  });

  const host = address.includes(":")
    ? address.slice(0, address.lastIndexOf(":"))
    : "127.0.0.1";
  return {
    address: `${host}:${boundPort}`,
    close: () =>
      new Promise<void>((resolve) => {
        server.tryShutdown(() => resolve());
      }),
  };
}
