import { DEFINITION_HASH } from "../harness/definitions.js";
import { createHash } from "node:crypto";

export function canonical(value: unknown): string {
  if (Array.isArray(value)) return "[" + value.map(canonical).join(",") + "]";
  if (value !== null && typeof value === "object")
    return (
      "{" +
      Object.entries(value)
        .filter(([, v]) => v !== undefined)
        .sort(([a], [b]) => Buffer.compare(Buffer.from(a), Buffer.from(b)))
        .map(([k, v]) => JSON.stringify(k) + ":" + canonical(v))
        .join(",") +
      "}"
    );
  const result = JSON.stringify(value);
  if (result === undefined) throw new Error("Non-JSON input");
  return result;
}
export const canonicalHash = (value: unknown) =>
  createHash("sha256")
    .update(
      canonical(value).replace(
        /[<>&\u2028\u2029]/g,
        (c) => "\\u" + c.charCodeAt(0).toString(16).padStart(4, "0"),
      ),
    )
    .digest("hex");
export function stableId(namespace: string, name: string) {
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      namespace,
    )
  )
    throw new Error("Invalid identity namespace");
  const hash = createHash("sha1")
    .update(Buffer.from(namespace.replaceAll("-", ""), "hex"))
    .update(name)
    .digest()
    .subarray(0, 16);
  hash[6] = (hash[6]! & 15) | 80;
  hash[8] = (hash[8]! & 63) | 128;
  const h = hash.toString("hex");
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}
export interface BusinessTool {
  name: string;
  description: string;
  parameters: Record<string, unknown>;
  contract_version: string;
  replay: "safe" | "unsafe";
  effect: "read" | "write" | "proposal";
  roles?: string[];
}
export interface FrozenManifest {
  pi_version: "1.5.0";
  definition_hash: string;
  trusted_extensions: { path: string; sha256: string }[];
  model_key: string;
  release_id?: string;
  context_window: number;
  max_tokens: number;
  max_micros: number;
  tools: BusinessTool[];
  skills: Record<string, unknown>[];
  params: Record<string, unknown>;
  sha256: string;
}
export function freezeManifest(
  input: Record<string, unknown>,
  tools: BusinessTool[],
): Readonly<FrozenManifest> {
  if (
    input.trusted_extensions !== undefined &&
    JSON.stringify(input.trusted_extensions) !== "[]"
  )
    throw new Error(
      "Trusted extensions are selected by the image, not requests",
    );
  if (typeof input.model_key !== "string" || !input.model_key.trim())
    throw new Error("manifest.model_key is required");
  const names = new Set<string>();
  for (const tool of tools) {
    if (names.has(tool.name)) throw new Error("duplicate tool");
    names.add(tool.name);
    if (
      !/^[a-zA-Z][a-zA-Z0-9_.-]{0,95}$/.test(tool.name) ||
      !tool.contract_version ||
      !["read", "write", "proposal"].includes(tool.effect) ||
      !["safe", "unsafe"].includes(tool.replay)
    )
      throw new Error("Invalid tool descriptor");
  }
  const integer = (key: string, fallback: number, min: number, max: number) => {
    const value = input[key] ?? fallback;
    if (
      !Number.isSafeInteger(value) ||
      Number(value) < min ||
      Number(value) > max
    )
      throw new Error("Invalid manifest " + key);
    return Number(value);
  };
  const params = input.params ?? {};
  if (params === null || Array.isArray(params) || typeof params !== "object")
    throw new Error("Invalid model params");
  const skills = input.skills ?? [];
  if (
    !Array.isArray(skills) ||
    skills.some(
      (s) =>
        !s ||
        typeof s !== "object" ||
        typeof s.skill_id !== "string" ||
        typeof s.version !== "string" ||
        typeof s.resource_hash !== "string",
    )
  )
    throw new Error("Invalid Skill pins");
  const body = {
    pi_version: "1.5.0" as const,
    definition_hash: DEFINITION_HASH,
    trusted_extensions: [],
    model_key: input.model_key,
    ...(typeof input.release_id === "string"
      ? { release_id: input.release_id }
      : {}),
    context_window: integer("context_window", 128000, 4096, 2000000),
    max_tokens: integer("max_tokens", 8192, 1, 65536),
    max_micros: integer("max_micros", 1000000, 1, Number.MAX_SAFE_INTEGER),
    tools: structuredClone(tools),
    skills: structuredClone(skills) as Record<string, unknown>[],
    params: structuredClone(params) as Record<string, unknown>,
  };
  if (body.max_tokens >= body.context_window)
    throw new Error("Model output exceeds context");
  function freeze<T>(value: T): T {
    if (value && typeof value === "object") {
      for (const child of Object.values(value)) freeze(child);
      Object.freeze(value);
    }
    return value;
  }
  return freeze({ ...body, sha256: canonicalHash(body) });
}

export function hasCurrentDefinitions(manifest: FrozenManifest): boolean {
  return (
    manifest.pi_version === "1.5.0" &&
    manifest.definition_hash === DEFINITION_HASH
  );
}
