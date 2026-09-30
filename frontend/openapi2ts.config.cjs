/* eslint-disable @typescript-eslint/no-require-imports -- openapi2ts 配置由 Node CommonJS 加载。 */
const { loadEnvConfig } = require("@next/env");
const { resolve } = require("node:path");

loadEnvConfig(resolve(__dirname, ".."));
module.exports = {
  schemaPath: `${process.env.LV_API_BASE_URL || "http://127.0.0.1:8080"}/swagger/doc.json`,
  serversPath: "./src/gen",
  projectName: "api",
  requestImportStatement: 'import { request } from "@/lib/request";',
  requestOptionsType: 'import("@/lib/request").RequestOptions',
  nullable: false,
  hook: {
    // 保留 Go schema 的包名，避免多个 ListResponse 被生成器折叠为同名类型。
    afterOpenApiDataInited(schema) {
      const rename = (name) => name.replaceAll(".", "_");
      const schemas = schema.components?.schemas;
      if (schemas)
        schema.components.schemas = Object.fromEntries(
          Object.entries(schemas).map(([name, value]) => [rename(name), value]),
        );
      const visit = (value) => {
        if (!value || typeof value !== "object") return;
        for (const [key, child] of Object.entries(value)) {
          if (
            key === "$ref" &&
            typeof child === "string" &&
            child.startsWith("#/components/schemas/")
          )
            value[key] =
              `#/components/schemas/${rename(child.slice("#/components/schemas/".length))}`;
          else visit(child);
        }
      };
      visit(schema);
      return schema;
    },
  },
};
