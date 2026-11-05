import { readFile, writeFile, mkdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import { buildApp, type AgentApplication } from "../src/http/app.js";
import {
  AuthorizedInputSchema,
  CreateRunSchema,
  CreateSessionSchema,
  ControlSchema,
  SessionViewSchema,
  RunSnapshotSchema,
  AgentStateSchema,
  PlanItemSchema,
  ProposalSchema,
  AcceptedRunSchema,
  ControlReceiptSchema,
  SessionListSchema,
  RunPageSchema,
  MessagePageSchema,
  PublicMessageSchema,
  CurrentSubmissionSchema,
} from "../contracts/agent.js";

const unavailable = async () => {
  throw new Error("schema export never executes application operations");
};
const service: AgentApplication = {
  ready: async () => false,
  createSession: unavailable,
  listSessions: unavailable,
  readSession: unavailable,
  createRun: unavailable,
  readRun: unavailable,
  command: unavailable,
  events: unavailable,
  close: async () => {},
  listRuns: unavailable,
  readSubmission: unavailable,
  currentSubmission: unavailable,
  listMessages: unavailable,
  readPublicMessage: unavailable,
};
const app = await buildApp({
  service,
  serviceToken: "offline-schema-only-not-an-accepted-service-token",
});
const spec = app.swagger();
await app.close();
const root = fileURLToPath(new URL("../", import.meta.url));
const check = process.argv.includes("--check");
const json = JSON.stringify(spec, null, 2) + "\n";
async function emit(path: string, value: string) {
  if (check) {
    if ((await readFile(path, "utf8")) !== value)
      throw new Error(`Generated contract differs: ${path}`);
  } else {
    await writeFile(path, value);
  }
}
await emit(root + "contracts/openapi.json", json);
type Schema = {
  type?: string;
  properties?: Record<string, Schema>;
  required?: string[] | undefined;
  additionalProperties?: Schema | boolean;
  anyOf?: Schema[];
  nullable?: boolean;
  items?: Schema;
};
const goSchemas = {
  AuthorizedInput: AuthorizedInputSchema,
  CreateSessionRequest: CreateSessionSchema,
  CreateRunRequest: CreateRunSchema,
  ControlRequest: ControlSchema,
  SessionView: SessionViewSchema,
  RunSnapshot: RunSnapshotSchema,
  AgentState: AgentStateSchema,
  PlanItem: PlanItemSchema,
  ProposalView: ProposalSchema,
  AcceptedRun: AcceptedRunSchema,
  ControlReceipt: ControlReceiptSchema,
  SessionPage: SessionListSchema,
  RunPage: RunPageSchema,
  MessagePage: MessagePageSchema,
  PublicMessage: PublicMessageSchema,
  CurrentSubmission: CurrentSubmissionSchema,
} satisfies Record<string, Schema>;
const goSchemaNames = new Map(
  Object.entries(goSchemas).map(([name, schema]) => [
    JSON.stringify(schema),
    name,
  ]),
);
const reusableSchemas = {
  AgentState: AgentStateSchema,
  ProposalView: ProposalSchema,
  MessagePage: MessagePageSchema,
};
function goType(s: Schema): string {
  if (s.type === "object" && s.properties) {
    for (const [name, schema] of Object.entries(reusableSchemas))
      if (JSON.stringify(s) === JSON.stringify(schema)) return name;
  }
  // Named DTOs preserve nested optionality through the public Swagger parser.
  const named = goSchemaNames.get(JSON.stringify(s));
  if (named) return named;
  if (s.anyOf)
    return s.anyOf.every((x) => x.type === "string" || x.type === "null")
      ? "string"
      : "json.RawMessage";
  switch (s.type) {
    case "string":
      return "string";
    case "integer":
      return "int64";
    case "number":
      return "float64";
    case "boolean":
      return "bool";
    case "array":
      if (!s.items) throw new Error("Missing items");
      return "[]" + goType(s.items);
    case "object":
      if (s.properties)
        return (
          "struct {\n" +
          Object.entries(s.properties)
            .map(([key, field]) => {
              const optional = !s.required?.includes(key);
              return ` ${goName(key)} ${optional ? "*" : ""}${goType(field)} \`json:"${key}${optional ? ",omitempty" : ""}"${goTags(field, optional)}\`\n`;
            })
            .join("") +
          "}"
        );
      return "json.RawMessage";
    default:
      return "json.RawMessage";
  }
}
function goTags(s: Schema, optional: boolean) {
  const t = goType(s);
  return (
    (t === "json.RawMessage"
      ? ' swaggertype:"object"'
      : t === "[]json.RawMessage"
        ? ' swaggertype:"array,object"'
        : "") +
    (s.nullable || s.anyOf?.some((option) => option.type === "null")
      ? ' extensions:"x-nullable"'
      : "") +
    (optional ? "" : ' binding:"required"')
  );
}
function goName(s: string) {
  return s
    .split("_")
    .map((x) => (x === "id" ? "ID" : x.charAt(0).toUpperCase() + x.slice(1)))
    .join("");
}
let go =
  '// Code generated from agent/contracts TypeBox schemas and route metadata. DO NOT EDIT.\npackage runtimeclient\n\nimport "encoding/json"\n\n';
for (const [name, schema] of Object.entries(goSchemas)) {
  const s = schema as Schema;
  go += `type ${name} struct {\n`;
  for (const [key, field] of Object.entries(s.properties ?? {})) {
    const optional = !s.required?.includes(key);
    const t = goType(field);
    go += ` ${goName(key)} ${optional ? "*" : ""}${t} \`json:"${key}${optional ? ",omitempty" : ""}"${goTags(field, optional)}\`\n`;
  }
  go += "}\n\n";
}
go +=
  "// OperationPaths is generated together with the request types.\nvar OperationPaths = map[string]string{\n";
for (const [path, item] of Object.entries(spec.paths ?? {})) {
  for (const method of ["get", "post", "delete", "patch", "put"] as const) {
    const op = item?.[method];
    if (op?.operationId)
      go += ` "${op.operationId}": "${method.toUpperCase()} ${path}",\n`;
  }
}
go += "}\n";
const destination = root + "../backend/internal/agent/adapter/runtimeclient";
await mkdir(destination, { recursive: true });
const formatted = execFileSync("gofmt", [], { input: go, encoding: "utf8" });
await emit(destination + "/generated.go", formatted);
console.log(
  check
    ? "Agent OpenAPI and Go contracts are current"
    : "Generated agent OpenAPI and Go request contracts",
);
