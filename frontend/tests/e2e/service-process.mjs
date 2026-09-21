import { spawn } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Playwright owns this process; this process owns exactly one temporary service.
const kind = process.argv[2];
if (kind !== "postgres" && kind !== "minio") throw new Error("Unknown E2E service");
const port =
  process.env[`LANVERSE_E2E_${kind.toUpperCase()}_PORT`] ??
  (kind === "postgres" ? "15432" : "19010");
if (!/^\d+$/.test(port) || Number(port) < 1 || Number(port) > 65535) {
  throw new Error("Invalid E2E service port");
}
const directory = await mkdtemp(join(tmpdir(), `lanverse-e2e-${kind}.`));
let child;
let stopping = false;
const stop = () => {
  stopping = true;
  child?.kill("SIGTERM");
};
process.on("SIGTERM", stop);
process.on("SIGINT", stop);

async function run(command, args, env = process.env) {
  if (stopping) return;
  await new Promise((resolve, reject) => {
    child = spawn(command, args, { stdio: "inherit", env });
    child.once("error", reject);
    child.once("close", (code) => {
      child = undefined;
      if (code === 0 || stopping) resolve();
      else reject(new Error(`${command} exited with status ${code}`));
    });
  });
}

try {
  if (kind === "postgres") {
    await run("initdb", [
      "-D",
      directory,
      "--auth=trust",
      "--encoding=UTF8",
      "--no-locale",
      "--username=lanverse_e2e",
    ]);
    await run("postgres", ["-D", directory, "-h", "127.0.0.1", "-p", port]);
  } else {
    await run(
      "minio",
      [
        "server",
        "--quiet",
        "--address",
        `127.0.0.1:${port}`,
        "--console-address",
        "127.0.0.1:0",
        directory,
      ],
      {
        ...process.env,
        MINIO_ROOT_USER: "lanverse-e2e",
        MINIO_ROOT_PASSWORD: "lanverse-e2e-only",
      },
    );
  }
} finally {
  await rm(directory, { recursive: true, force: true });
}
