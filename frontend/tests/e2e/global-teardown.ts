import { readFile, rm, unlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";

async function cleanRecordedDirectory(kind: "minio" | "postgres", port: string): Promise<void> {
  const temporaryRoot = path.resolve(tmpdir());
  const stateFile = path.join(temporaryRoot, `lanverse-e2e-${kind}-${port}.path`);
  let recorded: string;
  try {
    recorded = (await readFile(stateFile, "utf8")).trim();
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return;
    throw error;
  }
  const target = path.resolve(recorded);
  const expectedPrefix = `lanverse-e2e-${kind}.`;
  if (path.dirname(target) !== temporaryRoot || !path.basename(target).startsWith(expectedPrefix)) {
    throw new Error(`拒绝清理不可信的 E2E 临时目录：${target}`);
  }
  await rm(target, { force: true, recursive: true });
  await unlink(stateFile).catch((error: NodeJS.ErrnoException) => {
    if (error.code !== "ENOENT") throw error;
  });
}

export default async function globalTeardown(): Promise<void> {
  await Promise.all([
    cleanRecordedDirectory("postgres", process.env.LANVERSE_E2E_POSTGRES_PORT ?? "15432"),
    cleanRecordedDirectory("minio", process.env.LANVERSE_E2E_MINIO_PORT ?? "19010"),
  ]);
}
