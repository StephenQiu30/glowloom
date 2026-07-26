import path from "node:path";

const config = {
  schemaPath:
    process.env.LANVERSE_OPENAPI_URL ??
    "http://127.0.0.1:8000/openapi.json",
  serversPath: path.resolve(process.cwd(), "./src/services/generated"),
  namespace: "API",
  nullable: true,
  enumStyle: "string-literal",
  requestOptionsType: "RequestOptions",
  requestImportStatement:
    "import { request, type RequestOptions } from '@/lib/request';",
};

export default config;
