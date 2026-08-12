import type { OpenAPIObject } from "openapi3-ts";

const schemaPath = process.env.OPENAPI_SCHEMA_URL;

if (!schemaPath) {
  throw new Error("OPENAPI_SCHEMA_URL is required");
}

const HTTP_METHODS = [
  "get",
  "post",
  "put",
  "patch",
  "delete",
  "options",
  "head",
] as const;

function normalizeGeneratorSuccessResponses(
  openAPIData: OpenAPIObject
): OpenAPIObject {
  exposeConstAsEnum(openAPIData);
  for (const pathItem of Object.values(openAPIData.paths)) {
    if (!pathItem) continue;
    for (const method of HTTP_METHODS) {
      const responses = pathItem[method]?.responses;
      if (!responses || responses.default || responses["200"] || responses["201"])
        continue;
      const successStatus = Object.keys(responses).find((status) =>
        /^2\d\d$/.test(status)
      );
      if (successStatus) responses["200"] = responses[successStatus];
    }
  }
  return openAPIData;
}

function exposeConstAsEnum(value: unknown): void {
  if (typeof value !== "object" || value === null) return;
  const object = value as Record<string, unknown>;
  if ("const" in object && !("enum" in object)) object.enum = [object.const];
  for (const child of Object.values(object)) exposeConstAsEnum(child);
}

const config = {
  schemaPath,
  serversPath: "./src/api",
  projectName: ".",
  requestLibPath: "@/lib/request",
  requestOptionsType: "RequestOptions",
  requestImportStatement:
    "import request, { type RequestOptions } from '@/lib/request';",
  isCamelCase: true,
  nullable: true,
  enumStyle: "string-literal",
  hook: { afterOpenApiDataInited: normalizeGeneratorSuccessResponses },
};

export default config;
