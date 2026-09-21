import { generateService, type GenerateServiceProps } from "@umijs/openapi";

const schemaPath = process.env.OPENAPI_SCHEMA_URL;
if (!schemaPath || !/^https?:\/\//.test(schemaPath)) {
  throw new Error("OPENAPI_SCHEMA_URL 必须指定在线 Swagger / OpenAPI JSON 地址");
}

const config: GenerateServiceProps = {
  schemaPath,
  serversPath: "./src/api",
  projectName: "generated",
  namespace: "GeneratedAPI",
  requestImportStatement: 'import request, { type RequestOptions } from "@/lib/request";',
  requestOptionsType: "RequestOptions",
  nullable: false,
  hook: {
    afterOpenApiDataInited: (document) => {
      // umi-openapi does not resolve reusable parameter references itself.
      for (const item of Object.values(document.paths)) {
        if (!item) continue;
        for (const method of [
          "get",
          "post",
          "put",
          "patch",
          "delete",
          "head",
          "options",
        ] as const) {
          const operation = item[method];
          if (!operation) continue;
          operation.parameters = [...(item.parameters ?? []), ...(operation.parameters ?? [])].map(
            (parameter) => {
              if (!("$ref" in parameter)) return parameter;
              const prefix = "#/components/parameters/";
              if (!parameter.$ref.startsWith(prefix))
                throw new Error("Unsupported parameter reference");
              const resolved =
                document.components?.parameters?.[parameter.$ref.slice(prefix.length)];
              if (!resolved || "$ref" in resolved)
                throw new Error(`Unresolved parameter: ${parameter.$ref}`);
              return resolved;
            },
          );
        }
        delete item.parameters;
      }
      return document;
    },
    customFunctionName: (operation) => {
      if (!operation.operationId) throw new Error(`缺少 operationId: ${operation.path}`);
      return operation.operationId;
    },
    customFileNames: () => ["backend"],
  },
};

export default config;

// The upstream CLI catches errors without failing the process. Use its API so
// failed fetches and generation fail CI, and check HTTP status before writing.
async function main() {
  const response = await fetch(schemaPath!, { signal: AbortSignal.timeout(15_000) });
  if (!response.ok) throw new Error(`OpenAPI 下载失败: HTTP ${response.status}`);
  const document = await response.json();
  if ((!document.openapi && !document.swagger) || !document.paths) {
    throw new Error("响应不是有效的 Swagger / OpenAPI 文档");
  }
  await generateService(config);
}

main().catch((error: unknown) => {
  console.error(error instanceof Error ? error.message : "OpenAPI 生成失败");
  process.exitCode = 1;
});
