import path from "node:path";

const config = {
  schemaPath: path.resolve(process.cwd(), "../backend/openapi/openapi.json"),
  serversPath: path.resolve(process.cwd(), "./src/services/generated"),
  namespace: "API",
  nullable: true,
  enumStyle: "string-literal",
  requestOptionsType: "RequestOptions",
  requestImportStatement:
    "import { request, type RequestOptions } from '@/lib/request';",
};

export default config;
