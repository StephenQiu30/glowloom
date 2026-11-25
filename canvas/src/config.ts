export interface CanvasConfig {
  readonly host: string;
  readonly port: number;
  readonly grpcAddress: string;
  readonly token: string;
  readonly maxConcurrent: number;
}

function parsePositiveInt(
  raw: string | undefined,
  fallback: number,
  name: string,
  min: number,
  max: number,
): number {
  if (raw === undefined || raw.trim() === "") return fallback;
  const trimmed = raw.trim();
  const value = Number(trimmed);
  if (
    !/^\d+$/.test(trimmed) ||
    !Number.isSafeInteger(value) ||
    value < min ||
    value > max
  ) {
    throw new Error(`INVALID_${name}`);
  }
  return value;
}

export function loadConfig(env: NodeJS.ProcessEnv): CanvasConfig {
  const token = env.GLOWLOOM_COLLAB_RUNTIME_TOKEN?.trim() ?? "";
  if (Buffer.byteLength(token) < 32) {
    throw new Error("RUNTIME_TOKEN_REQUIRED");
  }
  const host = env.HOST?.trim() || "127.0.0.1";
  const port = parsePositiveInt(env.PORT, 4400, "PORT", 1, 65535);
  const grpcAddress = env.GLOWLOOM_CANVAS_GRPC_ADDR?.trim() || "0.0.0.0:4402";
  const maxConcurrent = parsePositiveInt(
    env.GLOWLOOM_CANVAS_MAX_CONCURRENT,
    4,
    "MAX_CONCURRENT",
    1,
    64,
  );
  return { host, port, grpcAddress, token, maxConcurrent };
}
