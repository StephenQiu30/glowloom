import { readFile, mkdir, writeFile } from "node:fs/promises";
import { existsSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { resolve, dirname } from "node:path";
import { randomBytes } from "node:crypto";
import { spawn } from "node:child_process";
import { startBridge } from "../src/server/bridge/service";
const workspace = resolve(fileURLToPath(new URL("..", import.meta.url)));
const local = resolve(workspace, ".local");
await mkdir(local, { recursive: true, mode: 0o700 });
async function secret(name: string) {
  const path = resolve(local, name);
  try {
    return (await readFile(path, "utf8")).trim();
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
    const value = randomBytes(32).toString("hex");
    await writeFile(path, value + "\n", { flag: "wx", mode: 0o600 });
    return value;
  }
}
const accessToken =
  process.env.GLOWLOOM_WORKSPACE_SESSION_SECRET ??
  (await secret("access-token"));
const bridgeToken = await secret("bridge-token");
const socket = resolve(local, "bridge.sock");
const appCli =
  "/Applications/ChatGPT.app/Contents/Resources/codex-cli/CodexCLI.app/Contents/MacOS/codex";
const codexBinary =
  process.env.GLOWLOOM_WORKSPACE_CODEX_BINARY ??
  (existsSync(appCli) ? appCli : "codex");
const current = JSON.parse(
  await readFile(resolve(workspace, "dist/online/current.json"), "utf8"),
);
const releaseRoot = resolve(workspace, "dist/online", current.releaseId);
const release = JSON.parse(
  await readFile(resolve(releaseRoot, "release.json"), "utf8"),
);
if (release.mode !== "online" || !release.entry)
  throw new Error("Expected online standalone release");
const port = process.env.WORKSPACE_PORT ?? "1130";
const origin =
  process.env.GLOWLOOM_WORKSPACE_ORIGIN ?? `http://127.0.0.1:${port}`;
const bridge = await startBridge({
  repo: dirname(workspace),
  storage: resolve(local, "bridge"),
  socket,
  token: bridgeToken,
  codexBinary,
});
const child = spawn(process.execPath, [resolve(releaseRoot, release.entry)], {
  cwd: releaseRoot,
  stdio: "inherit",
  env: {
    ...process.env,
    NODE_ENV: "production",
    PORT: port,
    HOSTNAME: "127.0.0.1",
    GLOWLOOM_WORKSPACE_ORIGIN: origin,
    GLOWLOOM_WORKSPACE_SESSION_SECRET: accessToken,
    GLOWLOOM_WORKSPACE_BRIDGE_SOCKET: socket,
    GLOWLOOM_WORKSPACE_BRIDGE_TOKEN: bridgeToken,
  },
});
console.log(`Online workspace: ${origin}${release.basePath}/`);
console.log(
  process.env.GLOWLOOM_WORKSPACE_SESSION_SECRET
    ? "Local access token: configured by environment (not printed)"
    : `Local access token file: ${resolve(local, "access-token")} (not printed)`,
);
console.log(
  "Real TTS generation is disabled. No schema is created at startup.",
);
let stopped = false;
const stop = async () => {
  if (stopped) return;
  stopped = true;
  child.kill("SIGTERM");
  await bridge.close();
};
process.on("SIGINT", () => void stop());
process.on("SIGTERM", () => void stop());
child.on("exit", () => {
  void stop();
});
