import path from "node:path";
import type { GenerateServiceProps } from "@umijs/openapi";

const config: GenerateServiceProps = {
  schemaPath: path.resolve("node_modules/.cache/openapi/swagger.json"),
  serversPath: path.resolve("src/api"),
  projectName: "",
  namespace: "API",
  requestLibPath:
    "import { request } from '@/lib/request'; import type { RequestOptions } from '@/lib/request';",
  requestOptionsType: "RequestOptions",
  hook: {
    afterOpenApiDataInited(schema) {
      // Umi strips Go package qualifiers. Retain module names for collisions.
      const definitions = schema.components?.schemas ?? {};
      const names = Object.keys(definitions);
      const shortName = (name: string) => name.split(".").at(-1)!;
      const renamed = new Map<string, string>();
      for (const name of names) {
        const short = shortName(name);
        const duplicate = names.some(
          (other) => other !== name && shortName(other) === short,
        );
        const moduleName = name.match(/internal_([^_]+)_/)?.[1];
        const target = duplicate
          ? `${moduleName![0].toUpperCase()}${moduleName!.slice(1)}${short}`
          : short;
        if ([...renamed.values()].includes(target))
          throw new Error(`Swagger type name collision: ${target}`);
        renamed.set(name, target);
      }
      function rewrite(value: unknown) {
        if (!value || typeof value !== "object") return;
        if ("$ref" in value && typeof value.$ref === "string") {
          const name = value.$ref.replace("#/components/schemas/", "");
          if (renamed.has(name))
            value.$ref = `#/components/schemas/${renamed.get(name)}`;
        }
        Object.values(value).forEach(rewrite);
      }
      rewrite(schema);
      schema.components ??= {};
      schema.components.schemas = Object.fromEntries(
        names.map((name) => [renamed.get(name), definitions[name]]),
      );
      return schema;
    },
    customFunctionName(operation) {
      if (!operation.operationId)
        throw new Error("Swagger operationId is required");
      return operation.operationId;
    },
    customFileNames: (operation) => [operation.tags?.[0] ?? "identity"],
    customType: (schema, namespace, getType) =>
      schema?.type === "string" && schema.format === "binary"
        ? "Blob"
        : getType(schema, namespace),
    customOptionsDefaultValue(operation) {
      const binary = Object.entries(operation.responses).some(
        ([status, response]) =>
          status.startsWith("2") &&
          Object.values(response.content ?? {}).some(
            (content) =>
              (content as { schema?: { format?: string } }).schema?.format ===
              "binary",
          ),
      );
      return binary ? { responseType: "blob" } : {};
    },
  },
};

export default config;
