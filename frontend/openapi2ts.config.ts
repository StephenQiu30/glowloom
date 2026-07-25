const config = {
  schemaPath: "../backend/openapi/openapi.json",
  serversPath: "./src/services/generated",
  namespace: "API",
  nullable: true,
  enumStyle: "string-literal",
  requestOptionsType: "RequestOptions",
  requestImportStatement:
    "import { request, type RequestOptions } from '@/lib/request';",
};

export default config;
