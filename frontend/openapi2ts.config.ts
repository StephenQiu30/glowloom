const schemaPath = process.env.OPENAPI_SCHEMA_URL;

if (!schemaPath) {
  throw new Error("OPENAPI_SCHEMA_URL is required");
}

const config = {
  schemaPath,
  serversPath: "./src/api",
  projectName: ".",
  requestLibPath: "@/lib/api-request",
  requestOptionsType: "RequestOptions",
  requestImportStatement:
    "import request, { type RequestOptions } from '@/lib/api-request';",
  isCamelCase: true,
  nullable: true,
  enumStyle: "string-literal",
};

export default config;
