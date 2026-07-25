import { readdir, readFile, writeFile } from "node:fs/promises";
import path from "node:path";

import { generateService } from "@umijs/openapi";

const frontendRoot = path.resolve(import.meta.dirname, "..");
const generatedRoot = path.join(frontendRoot, "src/services/generated");

await generateService({
  schemaPath: path.resolve(frontendRoot, "../backend/openapi/openapi.json"),
  serversPath: generatedRoot,
  namespace: "API",
  nullable: true,
  enumStyle: "string-literal",
  requestOptionsType: "RequestOptions",
  requestImportStatement:
    "import { request, type RequestOptions } from '@/lib/request';",
});

const apiRoot = path.join(generatedRoot, "api");
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
