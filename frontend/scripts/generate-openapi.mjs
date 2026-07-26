import { readdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";

import { generateService } from "@umijs/openapi";

const frontendRoot = path.resolve(import.meta.dirname, "..");
const generatedRoot = path.resolve(
  process.env.LANVERSE_OPENAPI_OUTPUT ??
    path.join(frontendRoot, "src/services/generated"),
);
const schemaPath =
  process.env.LANVERSE_OPENAPI_URL ??
  "http://127.0.0.1:8000/openapi.json";

if (path.basename(generatedRoot) !== "generated") {
  throw new Error("LANVERSE_OPENAPI_OUTPUT must name a generated directory");
}
const stagingRoot = `${generatedRoot}.tmp-${process.pid}`;
await rm(stagingRoot, { recursive: true, force: true });

try {
  await generateService({
    schemaPath,
    serversPath: stagingRoot,
    namespace: "API",
    nullable: true,
    enumStyle: "string-literal",
    requestOptionsType: "RequestOptions",
    requestImportStatement:
      "import { request, type RequestOptions } from '@/lib/request';",
  });

  const apiRoot = path.join(stagingRoot, "api");
  for (const filename of await readdir(apiRoot)) {
    if (!filename.endsWith(".ts")) continue;
    const target = path.join(apiRoot, filename);
    const source = await readFile(target, "utf8");
    const corrected = source
      .replaceAll("${confirm}", ":confirm")
      .replaceAll("${cancel}", ":cancel")
      .replaceAll("${retry}", ":retry");
    if (/\$\{(?:confirm|cancel|retry)\}/u.test(corrected)) {
      throw new Error(`unresolved action suffix in ${filename}`);
    }
    if (corrected !== source) await writeFile(target, corrected);
  }

  await rm(generatedRoot, { recursive: true, force: true });
  await rename(stagingRoot, generatedRoot);
} finally {
  await rm(stagingRoot, { recursive: true, force: true });
}
