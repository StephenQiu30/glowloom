import type { OpenAPIObject } from "openapi3-ts";
import { existsSync } from "node:fs";
import { resolve } from "node:path";

const schemaPath = process.env.OPENAPI_SCHEMA_URL;

if (!schemaPath) {
  throw new Error("OPENAPI_SCHEMA_URL is required");
}

const normalizedSchemaPath = resolve(process.cwd(), schemaPath);

if (!existsSync(normalizedSchemaPath)) {
  process.exitCode = 1;
  throw new Error(`OPENAPI_SCHEMA_URL does not exist: ${normalizedSchemaPath}`);
}

const config = {
  schemaPath: normalizedSchemaPath,
  serversPath: "./src/api",
  projectName: ".",
  requestLibPath: "import request, { type RequestOptions } from '@/lib/request'",
  requestOptionsType: "RequestOptions",
  requestImportStatement:
    "import request, { type RequestOptions } from '@/lib/request';",
  isCamelCase: true,
  nullable: true,
  enumStyle: "string-literal",
  hook: {
    afterOpenApiDataInited(data: OpenAPIObject) {
      return data;
    },
  },
};

export default config;
