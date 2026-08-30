import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const outputRoot = resolve(frontendRoot, "src/api");
const schemaOutput = resolve(outputRoot, "schema.d.ts");
const schemaInput =
  process.env.OPENAPI_SCHEMA_URL ?? "../backend/api/openapi/lanverse-public-api.json";
const remote = /^https?:\/\//u.test(schemaInput);
const resolvedInput = remote ? schemaInput : resolve(frontendRoot, schemaInput);
if (!remote && !existsSync(resolvedInput)) {
  throw new Error(`OPENAPI_SCHEMA_URL does not exist: ${resolvedInput}`);
}

mkdirSync(outputRoot, { recursive: true });
execFileSync(
  resolve(frontendRoot, "node_modules/.bin/openapi-typescript"),
  [resolvedInput, "-o", schemaOutput],
  { cwd: frontendRoot, stdio: "inherit" },
);

const document = remote
  ? await fetchJson(resolvedInput)
  : JSON.parse(readFileSync(resolvedInput, "utf8"));
const schemas = document?.components?.schemas;
if (!schemas || typeof schemas !== "object") {
  throw new Error("OpenAPI document has no component schemas");
}

const aliases = Object.keys(schemas).sort().map((name) => {
  if (!/^[A-Za-z_$][A-Za-z0-9_$]*$/u.test(name)) {
    throw new Error(`Schema name is not a TypeScript identifier: ${name}`);
  }
  return `    type ${name} = components["schemas"][${JSON.stringify(name)}];`;
});
writeFileSync(
  resolve(outputRoot, "typings.d.ts"),
  [
    'import type { components } from "./schema";',
    "",
    "declare global {",
    "  namespace API {",
    ...aliases,
    "  }",
    "}",
    "",
    "export {};",
    "",
  ].join("\n"),
);

async function fetchJson(url) {
  const response = await fetch(url);
  if (!response.ok) throw new Error(`OpenAPI request failed: ${response.status} ${url}`);
  return response.json();
}
