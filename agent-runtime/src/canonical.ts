import { createHash } from "node:crypto";

// Canonical JSON for protocol and commit hashes. JSON round-trips detach optional fields first.
export function canonicalJSON(value: unknown): string {
  if (value === null || typeof value !== "object") {
    const result = JSON.stringify(value);
    if (result === undefined) throw new Error("invalid_json_value");
    return result;
  }
  if (Array.isArray(value)) return `[${value.map(canonicalJSON).join(",")}]`;
  return `{${Object.keys(value)
    .sort()
    .map(
      (key) =>
        `${JSON.stringify(key)}:${canonicalJSON((value as Record<string, unknown>)[key])}`,
    )
    .join(",")}}`;
}

export function hashJSON(value: unknown): string {
  return createHash("sha256").update(canonicalJSON(value)).digest("hex");
}
