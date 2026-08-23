import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { basename, dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const HTTP_METHODS = new Set(["get", "post", "put", "patch", "delete"]);
const frontendRoot = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const outputRoot = resolve(frontendRoot, "src/api");
const schemaOutput = resolve(outputRoot, "schema.d.ts");
const schemaInput = process.env.OPENAPI_SCHEMA_URL;

if (!schemaInput) {
  throw new Error("OPENAPI_SCHEMA_URL is required");
}

const isRemoteSchema = /^https?:\/\//u.test(schemaInput);
const resolvedSchemaInput = isRemoteSchema
  ? schemaInput
  : resolve(frontendRoot, schemaInput);

if (!isRemoteSchema && !existsSync(resolvedSchemaInput)) {
  throw new Error(`OPENAPI_SCHEMA_URL does not exist: ${resolvedSchemaInput}`);
}

mkdirSync(outputRoot, { recursive: true });
execFileSync(
  resolve(frontendRoot, "node_modules/.bin/openapi-typescript"),
  [resolvedSchemaInput, "-o", schemaOutput],
  { cwd: frontendRoot, stdio: "inherit" },
);

const document = isRemoteSchema
  ? await fetchJson(resolvedSchemaInput)
  : JSON.parse(readFileSync(resolvedSchemaInput, "utf8"));

validateDocument(document);
const operations = collectOperations(document);
writeTypings(document, operations);
writeServices(operations);

async function fetchJson(url) {
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`OpenAPI request failed: ${response.status} ${url}`);
  }
  return response.json();
}

function validateDocument(value) {
  if (value?.openapi == null || value?.paths == null) {
    throw new Error("OPENAPI_SCHEMA_URL is not an OpenAPI document");
  }
  if (value?.components?.schemas == null) {
    throw new Error("OpenAPI document has no component schemas");
  }
}

function collectOperations(value) {
  const result = [];
  const operationIds = new Set();
  const functionNames = new Set();

  for (const [path, pathItem] of Object.entries(value.paths)) {
    for (const [method, operation] of Object.entries(pathItem)) {
      if (!HTTP_METHODS.has(method) || operation == null) continue;
      if (!operation.operationId) {
        throw new Error(`${method.toUpperCase()} ${path} has no operationId`);
      }
      if (operationIds.has(operation.operationId)) {
        throw new Error(`Duplicate operationId: ${operation.operationId}`);
      }
      operationIds.add(operation.operationId);

      const functionName = toCamelIdentifier(operation.operationId);
      if (functionNames.has(functionName)) {
        throw new Error(`Generated function name collision: ${functionName}`);
      }
      functionNames.add(functionName);

      const tags = operation.tags ?? [];
      if (tags.length !== 1) {
        throw new Error(`${operation.operationId} must have exactly one tag`);
      }

      const parameters = [...(pathItem.parameters ?? []), ...(operation.parameters ?? [])];
      if (parameters.some((parameter) => parameter.$ref != null)) {
        throw new Error(`${operation.operationId} uses unsupported parameter references`);
      }
      const requestParameters = parameters.filter(
        (parameter) => parameter.in === "path" || parameter.in === "query",
      );
      const unsupportedParameters = parameters.filter(
        (parameter) => parameter.in !== "path" && parameter.in !== "query",
      );
      if (unsupportedParameters.length > 0) {
        throw new Error(`${operation.operationId} uses unsupported header or cookie parameters`);
      }

      result.push({
        bodyType: resolveRequestBodyType(operation),
        functionName,
        method: method.toUpperCase(),
        operationId: operation.operationId,
        parameters: requestParameters,
        path,
        responseType: resolveResponseType(operation),
        summary: operation.summary ?? operation.operationId,
        tag: tags[0],
      });
    }
  }

  return result.sort((left, right) =>
    left.tag.localeCompare(right.tag) ||
    left.path.localeCompare(right.path) ||
    left.method.localeCompare(right.method),
  );
}

function resolveRequestBodyType(operation) {
  if (!operation.requestBody) return null;
  if (operation.requestBody.$ref != null) {
    throw new Error(`${operation.operationId} uses an unsupported request body reference`);
  }
  const schema = operation.requestBody.content?.["application/json"]?.schema;
  return resolveComponentType(schema, `${operation.operationId} request body`);
}

function resolveResponseType(operation) {
  const successResponse = Object.entries(operation.responses ?? {})
    .filter(([status]) => /^2\d\d$/u.test(status))
    .sort(([left], [right]) => Number(left) - Number(right))[0]?.[1];
  if (!successResponse) {
    throw new Error(`${operation.operationId} has no successful response`);
  }
  if (successResponse.$ref != null) {
    throw new Error(`${operation.operationId} uses an unsupported response reference`);
  }
  const schema = successResponse.content?.["application/json"]?.schema;
  if (schema != null && Object.keys(schema).length === 0) return null;
  return resolveComponentType(schema, `${operation.operationId} response`);
}

function resolveComponentType(schema, location) {
  const reference = schema?.$ref;
  const prefix = "#/components/schemas/";
  if (typeof reference !== "string" || !reference.startsWith(prefix)) {
    throw new Error(`${location} must reference a component schema`);
  }
  return sanitizeTypeIdentifier(reference.slice(prefix.length));
}

function sanitizeTypeIdentifier(value) {
  const hasTrailingUnderscore = value.endsWith("_");
  const parts = value.split(/_+/u).filter(Boolean);
  if (parts.length === 0) throw new Error(`Cannot generate a type name for ${value}`);
  const identifier = parts
    .map((part, index) => (index === 0 ? part : capitalize(part)))
    .join("");
  return hasTrailingUnderscore ? `${identifier}_` : identifier;
}

function toCamelIdentifier(value) {
  const parts = value.split(/_+/u).filter(Boolean);
  if (parts.length === 0) throw new Error(`Cannot generate a function name for ${value}`);
  return `${parts[0][0].toLowerCase()}${parts[0].slice(1)}${parts
    .slice(1)
    .map(capitalize)
    .join("")}`;
}

function capitalize(value) {
  return `${value[0].toUpperCase()}${value.slice(1)}`;
}

function tagIdentifier(value) {
  return value
    .split(/[^A-Za-z0-9]+/u)
    .filter(Boolean)
    .map((part, index) =>
      index === 0
        ? `${part[0].toLowerCase()}${part.slice(1)}`
        : capitalize(part),
    )
    .join("");
}

function writeTypings(value, apiOperations) {
  const aliases = new Map();
  for (const schemaName of Object.keys(value.components.schemas).sort()) {
    const alias = sanitizeTypeIdentifier(schemaName);
    const previous = aliases.get(alias);
    if (previous) {
      throw new Error(`Generated schema type collision: ${previous} and ${schemaName}`);
    }
    aliases.set(alias, schemaName);
  }

  const lines = [
    'import type { components, operations } from "./schema";',
    "",
    "type DefinedParameterGroup<T> = [NonNullable<T>] extends [never] ? {} : NonNullable<T>;",
    "type OperationParameters<Name extends keyof operations> =",
    '  DefinedParameterGroup<operations[Name]["parameters"]["path"]> &',
    '  DefinedParameterGroup<operations[Name]["parameters"]["query"]>;',
    "",
    "declare global {",
    "  namespace API {",
  ];

  for (const [alias, schemaName] of aliases) {
    lines.push(`    type ${alias} = components["schemas"][${JSON.stringify(schemaName)}];`);
  }

  const parameterOperations = apiOperations
    .filter((operation) => operation.parameters.length > 0)
    .sort((left, right) => left.functionName.localeCompare(right.functionName));
  for (const operation of parameterOperations) {
    lines.push(
      `    type ${operation.functionName}Params = OperationParameters<${JSON.stringify(operation.operationId)}>;`,
    );
  }

  lines.push("  }", "}", "", "export {};", "");
  writeFileSync(resolve(outputRoot, "typings.d.ts"), lines.join("\n"));
}

function writeServices(apiOperations) {
  const grouped = Map.groupBy(apiOperations, (operation) => operation.tag);
  const indexEntries = [];

  for (const [tag, taggedOperations] of [...grouped].sort(([left], [right]) =>
    left.localeCompare(right),
  )) {
    const identifier = tagIdentifier(tag);
    if (!identifier) throw new Error(`Cannot generate a service name for tag ${tag}`);
    const filename = `${identifier}.ts`;
    const lines = ['import request, { type RequestOptions } from "@/lib/request";', ""];
    for (const operation of taggedOperations) {
      lines.push(renderOperation(operation), "");
    }
    writeFileSync(resolve(outputRoot, filename), lines.join("\n"));
    indexEntries.push({ filename: basename(filename, ".ts"), identifier });
  }

  const indexLines = indexEntries.map(
    ({ filename, identifier }) => `import * as ${identifier} from "./${filename}";`,
  );
  indexLines.push(
    "",
    "const api = {",
    ...indexEntries.map(({ identifier }) => `  ${identifier},`),
    "};",
    "",
    "export default api;",
    "",
  );
  writeFileSync(resolve(outputRoot, "index.ts"), indexLines.join("\n"));
}

function renderOperation(operation) {
  const argumentsList = [];
  if (operation.parameters.length > 0) {
    argumentsList.push(`params: API.${operation.functionName}Params`);
  }
  if (operation.bodyType) argumentsList.push(`body: API.${operation.bodyType}`);
  argumentsList.push("options?: RequestOptions");

  const pathParameters = operation.parameters.filter((parameter) => parameter.in === "path");
  const hasQueryParameters = operation.parameters.some((parameter) => parameter.in === "query");
  let requestPath = operation.path;
  const prelude = [];
  if (pathParameters.length > 0) {
    const destructured = pathParameters
      .map((parameter, index) => `${parameter.name}: path${index}`)
      .join(", ");
    prelude.push(
      hasQueryParameters
        ? `  const { ${destructured}, ...queryParams } = params;`
        : `  const { ${destructured} } = params;`,
    );
    for (const [index, parameter] of pathParameters.entries()) {
      requestPath = requestPath.replace(`{${parameter.name}}`, `\${path${index}}`);
    }
    if (/(?<!\$)\{[^}]+\}/u.test(requestPath)) {
      throw new Error(`${operation.operationId} has an unresolved path parameter`);
    }
  }

  const responseType = operation.responseType ? `API.${operation.responseType}` : "unknown";
  const requestLines = [
    `  return request<${responseType}>(\`${requestPath}\`, {`,
    `    method: ${JSON.stringify(operation.method)},`,
  ];
  if (operation.bodyType) {
    requestLines.push('    headers: { "Content-Type": "application/json" },');
  }
  if (hasQueryParameters) {
    requestLines.push(
      pathParameters.length > 0 ? "    params: queryParams," : "    params,",
    );
  }
  if (operation.bodyType) requestLines.push("    data: body,");
  requestLines.push("    ...(options ?? {}),", "  });");

  return [
    `/** ${operation.summary} ${operation.method} ${operation.path} */`,
    `export async function ${operation.functionName}(`,
    ...argumentsList.map((argument) => `  ${argument},`),
    ") {",
    ...prelude,
    ...requestLines,
    "}",
  ].join("\n");
}
