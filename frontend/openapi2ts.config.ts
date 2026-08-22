import type { OpenAPIObject } from "openapi3-ts";
import { resolve } from "node:path";

const schemaPath = process.env.OPENAPI_SCHEMA_URL;

if (!schemaPath) {
  throw new Error("OPENAPI_SCHEMA_URL is required");
}

const normalizedSchemaPath = /^https?:\/\//.test(schemaPath)
  ? schemaPath
  : resolve(process.cwd(), schemaPath);

const config = {
  schemaPath: normalizedSchemaPath,
  serversPath: "./src/api",
  projectName: ".",
  requestLibPath: "@/lib/request",
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
